"""USC SSO + Duo MFA authentication flow targeting Brightspace.

Flow:
  1. GET brightspace.usc.edu/d2l/home → follows SAML redirect chain to login.usc.edu
     Captures: saml2Request JWT, secondVisitUrl, acsURL, spEntityID (from SAMLRequest XML)
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
  4. GET login.usc.edu/login/authduo?state=...&duo_code=... → saml2/continue
  5. POST saml2Request to SSORedirect (with acsURL + spEntityID) → SAMLResponse form
  6. POST SAMLResponse to Brightspace → session established; cookies saved on-device
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
# /d2l/home returns 200 with JS-triggered redirect; /d2l/login always fires the SAML chain directly
BRIGHTSPACE_LOGIN = f"{BRIGHTSPACE_BASE}/d2l/login?sessionExpired=0&target=%2fd2l%2fhome"
LOGIN_BASE = "https://login.usc.edu"

# Duo frameless client sends this akey (USC's Duo application key)
DUO_AKEY = "DAGV9PVTPM67AUM8L61P"

BROWSER_FEATURES = (
    '{"touch_supported":false,'
    '"platform_authenticator_status":"unavailable",'
    '"webauthn_supported":true,'
    '"screen_resolution_height":1440,'
    '"screen_resolution_width":3440,'
    '"screen_color_depth":24,'
    '"is_uvpa_available":false,'
    '"client_capabilities_uvpa":false}'
)

CLIENT_HINTS = (
    "eyJicmFuZHMiOlt7ImJyYW5kIjoiQ2hyb21pdW0iLCJ2ZXJzaW9uIjoiMTQ2In0seyJicmFuZCI6Ik5vd"
    "C1BLkJyYW5kIiwidmVyc2lvbiI6IjI0In0seyJicmFuZCI6Ikdvb2dsZSBDaHJvbWUiLCJ2ZXJzaW9uIj"
    "oiMTQ2In1dLCJmdWxsVmVyc2lvbkxpc3QiOlt7ImJyYW5kIjoiQ2hyb21pdW0iLCJ2ZXJzaW9uIjoiMTQ2"
    "LjAuNzY4MC4xNTMifSx7ImJyYW5kIjoiTm90LUEuQnJhbmQiLCJ2ZXJzaW9uIjoiMjQuMC4wLjAifSx7Im"
    "JyYW5kIjoiR29vZ2xlIENocm9tZSIsInZlcnNpb24iOiIxNDYuMC43NjgwLjE1MyJ9XSwibW9iaWxlIjpm"
    "YWxzZSwicGxhdGZvcm0iOiJtYWNPUyIsInBsYXRmb3JtVmVyc2lvbiI6IjE0LjguNCIsInVhRnVsbFZlcn"
    "Npb24iOiIxNDYuMC43NjgwLjE1MyJ9"
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
    """Handles the full USC SSO + Duo bypass-code login flow."""

    def __init__(self, http: httpx.Client) -> None:
        self._http = http

    # ------------------------------------------------------------------
    # Public entry point
    # ------------------------------------------------------------------

    def login(self, username: str, password: str, bypass_code: str) -> None:
        """Run the full auth flow. Mutates the http client's cookie jar."""
        logger.debug("Starting USC SSO login for %s", username)

        # Step 1: navigate to USC SSO entry → follow SAML redirect chain
        login_url = self._get_saml_login_url()
        logger.debug("SAML login URL: %s", login_url)

        # Step 2: POST credentials
        duo_oauth_url = self._post_credentials(login_url, username, password)
        logger.debug("Duo OAuth URL: %s", duo_oauth_url[:80])

        # Step 3: Duo MFA
        duo_code, state = self._do_duo_bypass(duo_oauth_url, bypass_code)
        logger.debug("duo_code=%s state=%s", duo_code[:8], state[:12])

        # Step 4: Exchange duo_code → SAML assertion
        saml_response, saml_post_url, relay_state = self._exchange_duo_code(duo_code, state)
        logger.debug("SAMLResponse obtained, posting to %s", saml_post_url)

        # Step 5: POST SAMLResponse → session established
        self._post_saml(saml_post_url, saml_response, relay_state)
        logger.debug("Login complete")

    # ------------------------------------------------------------------
    # Step 1: walk the SAML redirect chain
    # ------------------------------------------------------------------

    def _get_saml_login_url(self) -> str:
        """Navigate to Brightspace and follow SAML redirects to the USC login form.

        Flow (matching HAR):
          GET brightspace.usc.edu/d2l/home
          → 302 /d2l/login?sessionExpired=0&target=/d2l/home
          → 302 /d2l/lp/auth/saml/initiate-login?entityId=...&target=...
          → 302 login.usc.edu/sso/SSORedirect/...?SAMLRequest=...&RelayState=...
          → 200 login.usc.edu/sso/SSORedirect (SSORedirect page with hidden fields)
          JS on that page navigates to loginUrl which is login.usc.edu/login/login

        Captures (stored as instance attrs for post-Duo use):
          self._saml2_request   — JWT stored in localStorage by saml2-write.js
          self._second_visit_url — URL to POST saml2Request to after Duo
          self._acs_url          — AssertionConsumerServiceURL from SAMLRequest XML
          self._sp_entity_id     — Issuer from SAMLRequest XML
          self._relay_state      — RelayState from the SAML redirect (e.g. /d2l/home)
        """
        import base64
        import html as html_module
        import zlib
        from urllib.parse import parse_qs, urlparse

        # Walk the redirect chain manually so we can capture the SAMLRequest
        # before it disappears into the SSORedirect page's JavaScript.
        resp = self._http.get(
            BRIGHTSPACE_LOGIN,
            headers={**BASE_HEADERS, "Referer": BRIGHTSPACE_BASE + "/"},
            follow_redirects=False,
        )

        saml_request_b64: str | None = None
        relay_state: str | None = None

        # Follow redirects manually so we can capture SAMLRequest before it's consumed by JS
        for _ in range(10):
            loc = resp.headers.get("location", "")
            if not loc:
                # Stopped at a non-redirect — check we're somewhere useful
                break
            if loc.startswith("/"):
                parsed_cur = urlparse(str(resp.url))
                loc = f"{parsed_cur.scheme}://{parsed_cur.netloc}{loc}"

            # Capture SAMLRequest + RelayState when we see them in a redirect target
            parsed_loc = urlparse(loc)
            qs = parse_qs(parsed_loc.query, keep_blank_values=True)
            if "SAMLRequest" in qs and saml_request_b64 is None:
                saml_request_b64 = qs["SAMLRequest"][0]
                relay_state = qs.get("RelayState", [None])[0]
                logger.debug("Captured SAMLRequest (len=%d)", len(saml_request_b64))

            resp = self._http.get(loc, headers=BASE_HEADERS, follow_redirects=False)
            if resp.status_code == 200 and "login.usc.edu" in str(resp.url):
                break
            if resp.status_code == 200 and "brightspace.usc.edu" in str(resp.url):
                raise AuthError(
                    f"Landed back on Brightspace without going through SSO: {resp.url}"
                )

        resp.raise_for_status()
        final_url = str(resp.url)
        logger.debug("Landed at: %s", final_url[:100])

        # Parse acsURL and spEntityID from the SAMLRequest XML
        if saml_request_b64:
            try:
                xml = zlib.decompress(
                    base64.b64decode(saml_request_b64 + "=="), -zlib.MAX_WBITS
                ).decode()
                m_acs = re.search(r'AssertionConsumerServiceURL="([^"]+)"', xml)
                m_issuer = re.search(r'<[^:>]+:?Issuer[^>]*>([^<]+)</[^:>]+:?Issuer>', xml)
                self._acs_url = m_acs.group(1) if m_acs else None
                self._sp_entity_id = m_issuer.group(1).strip() if m_issuer else None
                logger.debug("acsURL=%s", self._acs_url)
                logger.debug("spEntityID=%s", self._sp_entity_id)
            except Exception as exc:
                logger.warning("Could not parse SAMLRequest XML: %s", exc)
                self._acs_url = None
                self._sp_entity_id = None
        else:
            self._acs_url = None
            self._sp_entity_id = None

        self._relay_state = relay_state

        # SSORedirect page: extract loginUrl, saml2Request, secondVisitUrl from hidden fields
        if "SSORedirect" in final_url or "sso/" in final_url:
            html_text = resp.text

            def _extract(field_id: str) -> str | None:
                m = re.search(
                    rf'id="{field_id}"[^>]*value="([^"]+)"'
                    rf'|value="([^"]+)"[^>]*id="{field_id}"',
                    html_text,
                )
                if not m:
                    return None
                return html_module.unescape(m.group(1) or m.group(2))

            login_url = _extract("loginUrl")
            if not login_url:
                raise AuthError("Could not extract loginUrl from SSORedirect page")

            self._saml2_request = _extract("saml2Request")
            self._second_visit_url = _extract("secondVisitUrl")
            logger.debug("saml2Request present: %s", bool(self._saml2_request))
            logger.debug("secondVisitUrl: %s", (self._second_visit_url or "")[:80])

            resp2 = self._http.get(login_url, headers=BASE_HEADERS, follow_redirects=True)
            resp2.raise_for_status()
            return str(resp2.url)

        if "/login/login" in final_url:
            self._saml2_request = None
            self._second_visit_url = None
            return final_url

        raise AuthError(f"Unexpected landing URL after SAML redirect: {final_url}")

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
        sid, at_prompt = self._post_duo_frameless_init(duo_host, sid, tx, xsrf)
        if not at_prompt:
            # Server sent us to preauth healthcheck first — walk that chain
            xsrf = self._duo_preauth_healthcheck(duo_host, sid, tx, xsrf)
        self._duo_get_prompt_data(duo_host, sid)
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
        """GET the Duo frameless page and extract _xsrf token.

        Duo sets a cookie named ``_xsrf|{sid}`` whose value is formatted as
        ``"base64token|timestamp|hmac"`` (quotes included).  The actual token
        to use is the base64-decoded first segment.
        """
        import base64

        url = f"https://{duo_host}/frame/frameless/v4/auth"
        resp = self._http.get(
            url,
            params={"sid": sid, "tx": tx},
            headers={**BASE_HEADERS, "Referer": LOGIN_BASE + "/"},
            follow_redirects=True,
        )
        resp.raise_for_status()

        # Duo's cookie name is "_xsrf|{sid}".  httpx may normalise the name, so
        # we search all Set-Cookie headers from this response AND iterate the
        # client cookie jar, both looking for anything starting with "_xsrf".
        def _decode_duo_xsrf(raw_value: str) -> str:
            """raw_value may be quoted and pipe-separated; decode the first segment."""
            raw_value = raw_value.strip('"').split("|")[0]
            try:
                return base64.b64decode(raw_value).decode()
            except Exception:
                return raw_value  # Already plain-text token

        # 1. Search Set-Cookie headers on the frameless GET response
        for h_name, h_val in resp.headers.multi_items():
            if h_name.lower() == "set-cookie" and "_xsrf" in h_val.lower():
                # Format: _xsrf|<sid>="base64|ts|hmac"; ...
                m = re.match(r'_xsrf[^=]*=("?[^;]+)', h_val)
                if m:
                    return _decode_duo_xsrf(m.group(1))

        # 2. Walk the client cookie jar (httpx may store it under a mangled name)
        for cookie in self._http.cookies.jar:
            if "_xsrf" in cookie.name.lower():
                return _decode_duo_xsrf(cookie.value)

        # 3. Fallback: embedded in page HTML (older Duo versions)
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
    ) -> tuple[str, bool]:
        """POST to frameless/v4/auth to initialize the session.

        Returns (sid, went_to_auth_prompt) where went_to_auth_prompt=True means
        the server skipped the preauth healthcheck and jumped straight to
        auth/prompt (bypass accounts or trusted sessions).
        """
        base = f"https://{duo_host}"
        url = f"{base}/frame/frameless/v4/auth"
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
                "screen_resolution_width": "3440",
                "screen_resolution_height": "1440",
                "extension_instance_key": "",
                "color_depth": "24",
                "has_touch_capability": "false",
                "ch_ua_error": "",
                "client_hints": CLIENT_HINTS,
                "is_cef_browser": "false",
                "is_ipad_os": "false",
                "is_ie_compatibility_mode": "",
                "is_user_verifying_platform_authenticator_available": "false",
                "user_verifying_platform_authenticator_available_error": "",
                "acting_ie_version": "",
                "react_support": "true",
                "react_support_error_message": "",
            },
            headers={
                **BASE_HEADERS,
                "Origin": base,
                "Referer": f"{base}/frame/frameless/v4/auth?sid={sid}&tx={tx}",
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
        else:
            new_sid = sid

        # Check if we went straight to auth/prompt (bypass/trusted) or healthcheck
        went_to_prompt = "auth/prompt" in location
        logger.debug(
            "frameless init redirect: %s (went_to_prompt=%s)", location[:80], went_to_prompt
        )
        return new_sid, went_to_prompt

    def _duo_preauth_healthcheck(self, duo_host: str, sid: str, tx: str, xsrf: str) -> str:
        """Walk the preauth healthcheck + return redirect chain.

        Returns the (possibly refreshed) xsrf token.

        Sequence (mirrors the HAR exactly):
          GET preauth/healthcheck → 200
          GET preauth/healthcheck/data → 200 (JSON)
          GET return → 303 → frameless URL
          GET frameless → 200 (React page reload)
          POST frameless → 302 → auth/prompt  (second auto-submit by React)
        """
        import base64

        def _decode_duo_xsrf(raw: str) -> str:
            raw = raw.strip('"').split("|")[0]
            try:
                return base64.b64decode(raw).decode()
            except Exception:
                return raw

        def _get_current_xsrf() -> str:
            for cookie in self._http.cookies.jar:
                if "_xsrf" in cookie.name.lower():
                    return _decode_duo_xsrf(cookie.value)
            return xsrf

        base = f"https://{duo_host}"
        frameless_url = f"{base}/frame/frameless/v4/auth"

        # 1. GET preauth/healthcheck
        self._http.get(
            f"{base}/frame/v4/preauth/healthcheck",
            params={"sid": sid},
            headers={**BASE_HEADERS, "Referer": f"{frameless_url}?sid={sid}"},
            follow_redirects=True,
        ).raise_for_status()

        # 2. GET healthcheck/data
        self._http.get(
            f"{base}/frame/v4/preauth/healthcheck/data",
            params={"sid": sid},
            headers={
                **BASE_HEADERS,
                "Accept": "application/json, text/plain, */*",
                "Referer": f"{base}/frame/v4/preauth/healthcheck?sid={sid}",
            },
            follow_redirects=True,
        ).raise_for_status()

        # 3. GET return → 303 → frameless URL (extract the redirect)
        resp = self._http.get(
            f"{base}/frame/v4/return",
            params={"sid": sid},
            headers={**BASE_HEADERS, "Referer": f"{base}/frame/v4/preauth/healthcheck?sid={sid}"},
            follow_redirects=False,
        )
        ret_loc = resp.headers.get("location", "")
        if ret_loc.startswith("/"):
            ret_loc = f"{base}{ret_loc}"
        frameless_loc = ret_loc or f"{frameless_url}?sid={sid}&tx={tx}"

        # 4. GET frameless (second page load — React rehydrates with healthcheck state)
        self._http.get(
            frameless_loc,
            headers={**BASE_HEADERS, "Referer": f"{base}/frame/v4/return?sid={sid}"},
            follow_redirects=False,
        )

        # 5. POST frameless (second auto-submit by React → redirects to auth/prompt)
        fresh_xsrf = _get_current_xsrf()
        resp2 = self._http.post(
            frameless_url,
            params={"sid": sid, "tx": tx},
            data={
                "tx": tx,
                "parent": "None",
                "_xsrf": fresh_xsrf,
                "version": "v4",
                "akey": DUO_AKEY,
                "has_session_trust_analysis_feature": "False",
                "session_trust_extension_id": "",
                "java_version": "",
                "flash_version": "",
                "screen_resolution_width": "3440",
                "screen_resolution_height": "1440",
                "extension_instance_key": "",
                "color_depth": "24",
                "has_touch_capability": "false",
                "ch_ua_error": "",
                "client_hints": CLIENT_HINTS,
                "is_cef_browser": "false",
                "is_ipad_os": "false",
                "is_ie_compatibility_mode": "",
                "is_user_verifying_platform_authenticator_available": "false",
                "user_verifying_platform_authenticator_available_error": "",
                "acting_ie_version": "",
                "react_support": "true",
                "react_support_error_message": "",
            },
            headers={
                **BASE_HEADERS,
                "Origin": base,
                "Referer": frameless_loc,
            },
            follow_redirects=False,
        )

        if resp2.status_code not in (301, 302, 303) or "auth/prompt" not in resp2.headers.get(
            "location", ""
        ):
            raise AuthError(
                f"Expected auth/prompt redirect from 2nd frameless POST, got "
                f"{resp2.status_code} -> {resp2.headers.get('location','')[:80]}"
            )

        # 6. Re-read xsrf in case it was rotated
        fresh_xsrf = _get_current_xsrf()
        logger.debug("xsrf after healthcheck cycle: %s", fresh_xsrf)
        return fresh_xsrf

    def _duo_get_prompt_data(self, duo_host: str, sid: str) -> None:
        """GET /frame/v4/auth/prompt/data — required before posting the passcode.

        This signals to Duo's backend that the React auth/prompt page has loaded
        and is ready to receive a factor submission.
        """
        base = f"https://{duo_host}"
        import urllib.parse
        bf = urllib.parse.quote(BROWSER_FEATURES)
        self._http.get(
            f"{base}/frame/v4/auth/prompt/data",
            params={
                "post_auth_action": "OIDC_EXIT",
                "browser_features": BROWSER_FEATURES,
                "sid": sid,
            },
            headers={
                **BASE_HEADERS,
                "Accept": "application/json, text/plain, */*",
                "Referer": f"{base}/frame/v4/auth/prompt?sid={sid}",
            },
            follow_redirects=True,
        ).raise_for_status()

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
    ) -> tuple[str, str, str | None]:
        """Exchange duo_code → SAML assertion.

        Actual flow (from HAR):
          1. GET /login/authduo → 302 → saml2/continue
          2. GET saml2/continue (follows redirect) → HTML page with JS that POSTs saml2Request
          3. POST secondVisitUrl (SSORedirect?ReqID=...) with saml2Request in body
             → HTML page with SAMLResponse auto-submit form targeting the SP
          4. Caller POSTs that SAMLResponse to the SP
        """
        # Step 1: GET authduo — follows to saml2/continue
        resp = self._http.get(
            f"{LOGIN_BASE}/login/authduo",
            params={"state": state, "duo_code": duo_code},
            headers={**BASE_HEADERS},
            follow_redirects=True,
        )
        resp.raise_for_status()
        logger.debug("authduo landed at: %s", str(resp.url)[:100])

        # Step 2: POST secondVisitUrl with saml2Request — this is what the JS does
        # saml2/continue page JS auto-submits a form with the stored saml2Request
        if not self._second_visit_url or not self._saml2_request:
            raise AuthError("Missing secondVisitUrl or saml2Request from initial SSO flow")

        # The browser JS (saml2-read.js) decodes the saml2Request JWT and appends
        # index, acsURL, spEntityID, and binding to the secondVisitUrl before POSTing.
        # Without these the Shibboleth IdP returns 500.
        # We captured acsURL and spEntityID dynamically from the SAMLRequest XML in step 1.
        import urllib.parse as _up
        ssored_url = f"{LOGIN_BASE}{self._second_visit_url}"
        parsed_ssored = _up.urlparse(ssored_url)
        qs = dict(_up.parse_qsl(parsed_ssored.query))
        qs["index"] = "null"
        qs["acsURL"] = self._acs_url or ""
        qs["spEntityID"] = self._sp_entity_id or ""
        qs["binding"] = ""
        ssored_url = _up.urlunparse(parsed_ssored._replace(query=_up.urlencode(qs)))
        saml2_continue_url = str(resp.url)
        logger.debug("POSTing saml2Request to: %s", ssored_url[:120])
        resp = self._http.post(
            ssored_url,
            data={"saml2Request": self._saml2_request},
            headers={
                **BASE_HEADERS,
                "Origin": LOGIN_BASE,
                "Referer": saml2_continue_url,
                "Content-Type": "application/x-www-form-urlencoded",
            },
            follow_redirects=True,
        )
        resp.raise_for_status()
        logger.debug("SSORedirect POST landed at: %s", str(resp.url)[:100])

        saml_response, post_url, relay_state = self._extract_saml_response(resp)
        return saml_response, post_url, relay_state

    def _extract_saml_response(self, resp: httpx.Response) -> tuple[str, str, str | None]:
        """Parse SAMLResponse, form action, and optional RelayState from an HTML response.

        The page may be a standard SAML POST binding form or auto-submit JS.
        Returns (saml_response, post_url, relay_state).
        """
        import html as html_module

        html = resp.text

        # Try form-based SAMLResponse
        match = re.search(
            r'<form[^>]+action=["\']([^"\']+)["\']',
            html,
            re.IGNORECASE,
        )
        post_url = html_module.unescape(match.group(1)) if match else None

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

        # Extract RelayState if present
        relay_match = re.search(
            r'name=["\']RelayState["\'][^>]*value=["\']([^"\']*)["\']'
            r'|value=["\']([^"\']*)["\'][^>]*name=["\']RelayState["\']',
            html,
            re.IGNORECASE,
        )
        relay_state = html_module.unescape(relay_match.group(1) or relay_match.group(2)) if relay_match else None

        if not post_url:
            raise AuthError("Could not find SAML POST URL in login response")

        # Make absolute
        if post_url.startswith("/"):
            # Could be on login.usc.edu or another USC service domain
            base = f"{urlparse(str(resp.url)).scheme}://{urlparse(str(resp.url)).netloc}"
            post_url = f"{base}{post_url}"

        return saml_response, post_url, relay_state

    # ------------------------------------------------------------------
    # Step 5: POST SAMLResponse to complete the login
    # ------------------------------------------------------------------

    def _post_saml(self, post_url: str, saml_response: str, relay_state: str | None = None) -> None:
        """POST SAMLResponse (and RelayState if present) to establish the session.

        Brightspace returns a 303 to /d2l/error/500 on success (yes, really) but the
        session cookies (d2lSessionVal, d2lSecureSessionVal) are set on the 303 itself.
        We must NOT follow the redirect — just verify the cookies are present.
        """
        data: dict[str, str] = {"SAMLResponse": saml_response}
        if relay_state:
            data["RelayState"] = relay_state
        resp = self._http.post(
            post_url,
            data=data,
            headers={
                **BASE_HEADERS,
                "Origin": LOGIN_BASE,
                "Referer": f"{LOGIN_BASE}/",
                "Content-Type": "application/x-www-form-urlencoded",
            },
            follow_redirects=False,
        )

        # Success = 3xx with d2lSessionVal cookie set
        if resp.status_code not in (301, 302, 303):
            raise AuthError(
                f"Expected redirect from SAMLResponse POST, got {resp.status_code}"
            )

        # Verify the session cookies landed
        session_cookie = self._http.cookies.get("d2lSessionVal")
        if not session_cookie:
            # Also check the response Set-Cookie headers directly
            set_cookies = resp.headers.get_list("set-cookie") if hasattr(resp.headers, "get_list") else [
                v for k, v in resp.headers.items() if k.lower() == "set-cookie"
            ]
            has_session = any("d2lSessionVal" in c for c in set_cookies)
            if not has_session:
                raise AuthError("SAMLResponse POST did not set d2lSessionVal — login may have failed")

        logger.debug("Session established (d2lSessionVal present)")
