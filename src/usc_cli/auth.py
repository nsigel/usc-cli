"""USC Brightspace SSO + Duo MFA authentication flow.

Flow:
  1. GET brightspace.usc.edu/d2l/home → follow SAML chain to login.usc.edu
  2. POST /login/authuserpassword (j_username, j_password) → redirect to Duo OAuth
  3. Duo frameless v4:
       a. Extract sid + tx from OAuth redirect
       b. GET frameless auth page → parse _xsrf
       c. POST frameless init (tx, _xsrf, akey) → healthcheck redirect
       d. GET preauth/healthcheck, GET return, follow to auth/prompt
       e. GET prompt/data to confirm Passcode factor available
       f. POST /frame/v4/prompt with bypass passcode → get txid
       g. POST /frame/v4/status → confirm allow
       h. POST /frame/v4/oidc/exit → get duo_code + state
  4. GET login.usc.edu/login/authduo?state=...&duo_code=... → SAML chain
  5. GET saml2/continue → parse SAMLResponse from HTML form
  6. POST brightspace samlLogin.d2l with SAMLResponse → session established
"""

from __future__ import annotations

import logging
import re
import time
from urllib.parse import parse_qs, urlparse

import httpx

logger = logging.getLogger(__name__)

# Static USC/Duo constants
BRIGHTSPACE_BASE = "https://brightspace.usc.edu"
LOGIN_BASE = "https://login.usc.edu"

# Duo frameless client sends this akey (USC's Duo application key)
DUO_AKEY = "DAGV9PVTPM67AUM8L61P"

BROWSER_FEATURES = (
    '{"touch_supported":false,'
    '"platform_authenticator_status":"unavailable",'
    '"webauthn_supported":true,'
    '"screen_resolution_height":1440,'
    '"screen_resolution_width":2560,'
    '"screen_color_depth":24,'
    '"is_uvpa_available":false,'
    '"client_capabilities_uvpa":false}'
)

USER_AGENT = (
    "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) "
    "AppleWebKit/537.36 (KHTML, like Gecko) "
    "Chrome/124.0.0.0 Safari/537.36"
)

BASE_HEADERS = {
    "User-Agent": USER_AGENT,
    "Accept-Language": "en-US,en;q=0.9",
}


class AuthError(Exception):
    """Raised when the auth flow fails."""


class USCAuth:
    """Handles the full SSO + Duo bypass-code login flow for Brightspace."""

    def __init__(self, http: httpx.Client) -> None:
        self._http = http

    # ------------------------------------------------------------------
    # Public entry point
    # ------------------------------------------------------------------

    def login(self, username: str, password: str, bypass_code: str) -> None:
        """Run the full auth flow. Mutates the http client's cookie jar."""
        logger.debug("Starting USC SSO login for %s", username)

        # Step 1: navigate to Brightspace → follow SAML redirect chain → arrive
        # at login.usc.edu/login/login with service + goto params
        login_url = self._get_saml_login_url()
        logger.debug("SAML login URL: %s", login_url)

        # Step 2: POST credentials
        duo_oauth_url = self._post_credentials(login_url, username, password)
        logger.debug("Duo OAuth URL: %s", duo_oauth_url[:80])

        # Step 3: Duo frameless v4 MFA with bypass code
        duo_code, state = self._do_duo_bypass(duo_oauth_url, bypass_code)
        logger.debug("duo_code=%s state=%s", duo_code[:8], state[:12])

        # Step 4: Exchange duo_code → SAML assertion
        saml_response, saml_post_url = self._exchange_duo_code(duo_code, state)
        logger.debug("SAMLResponse obtained, posting to %s", saml_post_url)

        # Step 5: POST SAMLResponse → Brightspace session cookies
        self._post_saml(saml_post_url, saml_response)
        logger.debug("Login complete")

    # ------------------------------------------------------------------
    # Step 1: walk the SAML redirect chain
    # ------------------------------------------------------------------

    def _get_saml_login_url(self) -> str:
        """Navigate to Brightspace and follow SAML redirects to the USC login page."""
        resp = self._http.get(
            f"{BRIGHTSPACE_BASE}/d2l/home",
            headers=BASE_HEADERS,
            follow_redirects=True,
        )
        resp.raise_for_status()

        # After the redirect chain we should be at login.usc.edu
        final_url = str(resp.url)
        if "login.usc.edu" not in final_url:
            raise AuthError(f"Unexpected landing URL after SAML redirect: {final_url}")

        return final_url

    # ------------------------------------------------------------------
    # Step 2: POST username/password
    # ------------------------------------------------------------------

    def _post_credentials(self, login_url: str, username: str, password: str) -> str:
        """POST to /login/authuserpassword and return the Duo OAuth redirect URL."""
        # The action URL is always the same regardless of goto params
        post_url = f"{LOGIN_BASE}/login/authuserpassword"

        resp = self._http.post(
            post_url,
            data={
                "j_username": username,
                "j_password": password,
                "_eventId_proceed": "",
            },
            headers={
                **BASE_HEADERS,
                "Origin": LOGIN_BASE,
                "Referer": login_url,
                "Content-Type": "application/x-www-form-urlencoded",
            },
            follow_redirects=False,
        )

        # Expect a redirect to Duo OAuth
        if resp.status_code not in (301, 302, 303):
            # Could be a bad password — check for error text
            if "incorrect" in resp.text.lower() or "invalid" in resp.text.lower():
                raise AuthError("Invalid username or password")
            raise AuthError(
                f"Expected redirect after credentials POST, got {resp.status_code}"
            )

        location = resp.headers.get("location", "")
        if not location:
            raise AuthError("No Location header after credentials POST")

        # Make absolute if relative
        if location.startswith("/"):
            location = f"{LOGIN_BASE}{location}"

        if "duosecurity.com" not in location:
            raise AuthError(f"Expected Duo redirect, got: {location[:100]}")

        return location

    # ------------------------------------------------------------------
    # Step 3: Duo frameless v4 with bypass code
    # ------------------------------------------------------------------

    def _do_duo_bypass(self, duo_oauth_url: str, bypass_code: str) -> tuple[str, str]:
        """Drive the Duo frameless v4 flow with a bypass code.

        Returns (duo_code, state) to hand back to login.usc.edu.
        """
        duo_host, sid, tx = self._init_duo_session(duo_oauth_url)
        xsrf = self._get_duo_xsrf(duo_host, sid, tx)
        sid = self._post_duo_frameless_init(duo_host, sid, tx, xsrf)
        self._duo_preauth_healthcheck(duo_host, sid)
        txid = self._post_duo_prompt(duo_host, sid, xsrf, bypass_code)
        self._poll_duo_status(duo_host, sid, txid)
        duo_code, state = self._duo_oidc_exit(duo_host, sid, txid, xsrf)
        return duo_code, state

    def _init_duo_session(self, duo_oauth_url: str) -> tuple[str, str, str]:
        """GET Duo OAuth URL → follow 303 → extract duo_host, sid, tx from final URL."""
        resp = self._http.get(
            duo_oauth_url,
            headers={**BASE_HEADERS, "Referer": LOGIN_BASE + "/"},
            follow_redirects=False,
        )

        if resp.status_code not in (301, 302, 303):
            raise AuthError(f"Expected Duo OAuth redirect, got {resp.status_code}")

        location = resp.headers.get("location", "")
        # Make absolute if needed
        parsed_oauth = urlparse(duo_oauth_url)
        duo_host = parsed_oauth.netloc  # e.g. api-22627695.duosecurity.com

        if location.startswith("/"):
            frameless_url = f"https://{duo_host}{location}"
        else:
            frameless_url = location
            duo_host = urlparse(frameless_url).netloc

        # Parse sid and tx from frameless URL
        parsed = urlparse(frameless_url)
        params = parse_qs(parsed.query)

        sid = params.get("sid", [None])[0]
        tx = params.get("tx", [None])[0]

        if not sid or not tx:
            raise AuthError(f"Could not extract sid/tx from Duo URL: {frameless_url[:100]}")

        logger.debug("Duo host=%s sid=%s", duo_host, sid)
        return duo_host, sid, tx

    def _get_duo_xsrf(self, duo_host: str, sid: str, tx: str) -> str:
        """GET the Duo frameless page and extract _xsrf token."""
        url = f"https://{duo_host}/frame/frameless/v4/auth"
        resp = self._http.get(
            url,
            params={"sid": sid, "tx": tx},
            headers={**BASE_HEADERS, "Referer": LOGIN_BASE + "/"},
            follow_redirects=True,
        )
        resp.raise_for_status()

        # _xsrf is set as a cookie by Duo
        xsrf = self._http.cookies.get("_xsrf", domain=duo_host)
        if xsrf:
            return xsrf

        # Fallback: parse from page HTML (some versions embed it)
        match = re.search(r'["\']_xsrf["\']\s*[,:\s]+["\']([\w]+)["\']', resp.text)
        if match:
            return match.group(1)

        # Last resort: try meta tag or hidden input
        match = re.search(
            r'name=["\']_xsrf["\'][^>]*value=["\']([\w]+)["\']'
            r'|value=["\']([\w]+)["\'][^>]*name=["\']_xsrf["\']',
            resp.text,
        )
        if match:
            return match.group(1) or match.group(2)

        raise AuthError("Could not extract _xsrf from Duo frameless page")

    def _post_duo_frameless_init(
        self, duo_host: str, sid: str, tx: str, xsrf: str
    ) -> str:
        """POST to frameless/v4/auth to initialize the session.

        Returns the sid (may be updated in redirect location).
        """
        url = f"https://{duo_host}/frame/frameless/v4/auth"
        resp = self._http.post(
            url,
            params={"sid": sid, "tx": tx},
            data={
                "tx": tx,
                "parent": "None",
                "_xsrf": xsrf,
                "version": "v4",
                "akey": DUO_AKEY,
                "has_session_trust_analysis_feature": "False",
                "session_trust_extension_id": "",
                "java_version": "",
                "flash_version": "",
                "screen_resolution_width": "2560",
                "screen_resolution_height": "1440",
                "extension_instance_key": "",
                "color_depth": "24",
                "has_touch_capability": "false",
                "ch_ua_error": "",
                "is_cef_browser": "false",
                "is_ipad_os": "false",
                "is_user_verifying_platform_authenticator_available": "false",
                "react_support": "true",
                "react_support_error_message": "",
            },
            headers={
                **BASE_HEADERS,
                "Origin": f"https://{duo_host}",
                "Referer": f"https://{duo_host}/frame/frameless/v4/auth?sid={sid}&tx={tx}",
            },
            follow_redirects=False,
        )

        if resp.status_code not in (301, 302, 303):
            raise AuthError(
                f"Expected redirect from Duo frameless init, got {resp.status_code}"
            )

        location = resp.headers.get("location", "")
        # Extract updated sid if present
        if "sid=" in location:
            params = parse_qs(urlparse(location).query)
            new_sid = params.get("sid", [sid])[0]
            return new_sid

        return sid

    def _duo_preauth_healthcheck(self, duo_host: str, sid: str) -> None:
        """Walk the preauth healthcheck + return redirect chain."""
        base = f"https://{duo_host}"

        # GET preauth/healthcheck → might redirect
        resp = self._http.get(
            f"{base}/frame/v4/preauth/healthcheck",
            params={"sid": sid},
            headers={**BASE_HEADERS, "Referer": f"{base}/frame/frameless/v4/auth?sid={sid}"},
            follow_redirects=True,
        )
        resp.raise_for_status()

        # GET return (navigates back to frameless which then lands at auth/prompt)
        resp = self._http.get(
            f"{base}/frame/v4/return",
            params={"sid": sid},
            headers={**BASE_HEADERS, "Referer": f"{base}/frame/v4/preauth/healthcheck?sid={sid}"},
            follow_redirects=True,
        )
        resp.raise_for_status()

    def _post_duo_prompt(
        self, duo_host: str, sid: str, xsrf: str, bypass_code: str
    ) -> str:
        """POST the bypass code to /frame/v4/prompt. Returns txid."""
        base = f"https://{duo_host}"

        resp = self._http.post(
            f"{base}/frame/v4/prompt",
            data={
                "passcode": bypass_code,
                "device": "null",
                "factor": "Passcode",
                "postAuthDestination": "OIDC_EXIT",
                "browser_features": BROWSER_FEATURES,
                "sid": sid,
            },
            headers={
                **BASE_HEADERS,
                "Origin": base,
                "Referer": f"{base}/frame/v4/auth/prompt?sid={sid}",
                "X-Xsrftoken": xsrf,
                "Content-Type": "application/x-www-form-urlencoded;charset=UTF-8",
                "Accept": "*/*",
            },
        )
        resp.raise_for_status()

        data = resp.json()
        if data.get("stat") != "OK":
            msg = data.get("message") or data.get("response", {}).get("message", "unknown")
            raise AuthError(f"Duo prompt rejected: {msg}")

        txid = data.get("response", {}).get("txid")
        if not txid:
            raise AuthError(f"No txid in Duo prompt response: {data}")

        return txid

    def _poll_duo_status(
        self,
        duo_host: str,
        sid: str,
        txid: str,
        max_wait: int = 30,
        interval: float = 1.5,
    ) -> None:
        """Poll /frame/v4/status until allowed or timeout."""
        base = f"https://{duo_host}"
        deadline = time.monotonic() + max_wait

        while time.monotonic() < deadline:
            resp = self._http.post(
                f"{base}/frame/v4/status",
                data={"txid": txid, "sid": sid},
                headers={
                    **BASE_HEADERS,
                    "Origin": base,
                    "Referer": f"{base}/frame/v4/auth/prompt?sid={sid}",
                    "Accept": "*/*",
                },
            )
            resp.raise_for_status()

            data = resp.json()
            if data.get("stat") != "OK":
                raise AuthError(f"Duo status error: {data}")

            result = data.get("response", {}).get("result", "").lower()
            status_code = data.get("response", {}).get("status_code", "").lower()

            if result == "success" or status_code == "allow":
                logger.debug("Duo MFA approved")
                return

            if result == "failure" or status_code == "deny":
                reason = data.get("response", {}).get("reason", "denied")
                raise AuthError(f"Duo MFA denied: {reason}")

            # Still pending — wait and retry
            time.sleep(interval)

        raise AuthError("Duo MFA timed out waiting for approval")

    def _duo_oidc_exit(
        self, duo_host: str, sid: str, txid: str, xsrf: str
    ) -> tuple[str, str]:
        """POST to /frame/v4/oidc/exit to complete Duo OIDC flow.

        Returns (duo_code, state) from the redirect back to login.usc.edu.
        """
        base = f"https://{duo_host}"

        resp = self._http.post(
            f"{base}/frame/v4/oidc/exit",
            data={
                "sid": sid,
                "txid": txid,
                "factor": "Bypass Code",
                "device_key": "",
                "_xsrf": xsrf,
                "dampen_choice": "true",
            },
            headers={
                **BASE_HEADERS,
                "Origin": base,
                "Referer": f"{base}/frame/v4/auth/prompt?sid={sid}",
            },
            follow_redirects=False,
        )

        if resp.status_code not in (301, 302, 303):
            raise AuthError(f"Expected redirect from oidc/exit, got {resp.status_code}")

        location = resp.headers.get("location", "")
        if not location:
            raise AuthError("No Location header from oidc/exit")

        # Make absolute
        if location.startswith("/"):
            location = f"{base}{location}"

        parsed = urlparse(location)
        params = parse_qs(parsed.query)

        duo_code = params.get("duo_code", [None])[0]
        state = params.get("state", [None])[0]

        if not duo_code or not state:
            raise AuthError(f"Missing duo_code/state in oidc/exit redirect: {location}")

        return duo_code, state

    # ------------------------------------------------------------------
    # Step 4: exchange duo_code back to USC login
    # ------------------------------------------------------------------

    def _exchange_duo_code(
        self, duo_code: str, state: str
    ) -> tuple[str, str]:
        """GET /login/authduo and follow SAML chain to get SAMLResponse + post URL."""
        # GET authduo — follows redirect chain to saml2/continue
        resp = self._http.get(
            f"{LOGIN_BASE}/login/authduo",
            params={"state": state, "duo_code": duo_code},
            headers={**BASE_HEADERS},
            follow_redirects=True,
        )
        resp.raise_for_status()

        # We should be at saml2/continue or the SSORedirect that returns a form
        # The final response should be a page with a SAMLResponse form
        # (either auto-submitted JS or a plain form)
        saml_response, post_url = self._extract_saml_response(resp)
        return saml_response, post_url

    def _extract_saml_response(self, resp: httpx.Response) -> tuple[str, str]:
        """Parse SAMLResponse and form action from an HTML response.

        The page may be a standard SAML POST binding form or auto-submit JS.
        """
        html = resp.text

        # Try form-based SAMLResponse
        match = re.search(
            r'<form[^>]+action=["\']([^"\']+)["\']',
            html,
            re.IGNORECASE,
        )
        post_url = match.group(1) if match else None

        match = re.search(
            r'name=["\']SAMLResponse["\'][^>]*value=["\']([^"\']+)["\']'
            r'|value=["\']([^"\']+)["\'][^>]*name=["\']SAMLResponse["\']',
            html,
            re.IGNORECASE,
        )
        if match:
            saml_response = (match.group(1) or match.group(2)).strip()
        else:
            # Try JSON/JS embedded SAMLResponse
            match = re.search(r'"SAMLResponse"\s*:\s*"([^"]+)"', html)
            if match:
                saml_response = match.group(1)
            else:
                raise AuthError(
                    f"Could not find SAMLResponse in page at {resp.url}"
                )

        if not post_url:
            # Default Brightspace SAML endpoint
            post_url = f"{BRIGHTSPACE_BASE}/d2l/lp/auth/login/samlLogin.d2l"

        # Make absolute
        if post_url.startswith("/"):
            # Could be on login.usc.edu or brightspace.usc.edu
            base = f"{urlparse(str(resp.url)).scheme}://{urlparse(str(resp.url)).netloc}"
            post_url = f"{base}{post_url}"

        return saml_response, post_url

    # ------------------------------------------------------------------
    # Step 5: POST SAMLResponse to Brightspace
    # ------------------------------------------------------------------

    def _post_saml(self, post_url: str, saml_response: str) -> None:
        """POST SAMLResponse to Brightspace to establish the session."""
        resp = self._http.post(
            post_url,
            data={"SAMLResponse": saml_response},
            headers={
                **BASE_HEADERS,
                "Origin": LOGIN_BASE,
                "Referer": f"{LOGIN_BASE}/",
                "Content-Type": "application/x-www-form-urlencoded",
            },
            follow_redirects=True,
        )
        resp.raise_for_status()

        # Verify we landed somewhere sensible on Brightspace
        if "brightspace.usc.edu" not in str(resp.url):
            raise AuthError(
                f"SAML POST landed on unexpected URL: {resp.url}"
            )

        # Check we're not on a login/error page
        if "/d2l/login" in str(resp.url) or "/d2l/lp/auth" in str(resp.url):
            raise AuthError("SAML login failed — still on auth page after SAMLResponse POST")

        logger.debug("Session established at %s", resp.url)
