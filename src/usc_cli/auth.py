"""
USC Brightspace authentication flow.

Implements headless SAML 2.0 SSO + Duo MFA (bypass code) login.

Flow:
  1. GET  brightspace.usc.edu/d2l/home  → 302 → SAML initiation
  2. GET  /d2l/lp/auth/saml/initiate-login → 302 → login.usc.edu SSO redirect
  3. GET  login.usc.edu SSO redirect    → extracts login page URL + goto param
  4. POST login.usc.edu/login/authuserpassword  (j_username, j_password)
       → 302 → Duo OAuth authorize URL (contains sid + tx JWT)
  5. GET  duosecurity.com/oauth/v1/authorize  → 303 → frameless/v4/auth?sid=&tx=
  6. GET  frameless/v4/auth  → extract _xsrf cookie
  7. POST frameless/v4/auth  (tx, parent=None, _xsrf, browser hints)
       → 303 → /frame/v4/preauth/healthcheck?sid=
  8. GET  /frame/v4/auth/prompt/data  (get prompt context)
  9. POST /frame/v4/prompt  (factor=Passcode, passcode=<bypass_code>, sid)
       → {txid: "..."}
 10. POST /frame/v4/status  (txid, sid)  → poll until result.status == "allow"
 11. POST /frame/v4/oidc/exit  (sid, txid, factor="Bypass Code", _xsrf)
       → 303 → login.usc.edu/login/authduo?state=...&duo_code=...
 12. GET  /login/authduo?state=...&duo_code=...
       → 302 → /sso/saml2/continue/...
 13. GET  /sso/saml2/continue/...
       → page with JS form that POSTs SAMLResponse
 14. POST brightspace.usc.edu/d2l/lp/auth/login/samlLogin.d2l (SAMLResponse)
       → 302 → /d2l/home  (session established, d2lSessionVal cookie set)
"""

from __future__ import annotations

import re
import time
import urllib.parse

import httpx

DUO_HOST = "api-22627695.duosecurity.com"
LOGIN_HOST = "login.usc.edu"
BRIGHTSPACE_HOST = "brightspace.usc.edu"

# Minimal browser fingerprint — Duo validates these are present but doesn't
# actually check the values for bypass-code flows.
_BROWSER_FEATURES = (
    '{"touch_supported":false,'
    '"platform_authenticator_status":"unavailable",'
    '"webauthn_supported":true,'
    '"screen_resolution_height":1080,'
    '"screen_resolution_width":1920,'
    '"screen_color_depth":24,'
    '"is_uvpa_available":false,'
    '"client_capabilities_uvpa":false}'
)

_USER_AGENT = (
    "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) "
    "AppleWebKit/537.36 (KHTML, like Gecko) "
    "Chrome/122.0.0.0 Safari/537.36"
)


class AuthError(Exception):
    """Raised when authentication fails at any step."""


def _raise_for(resp: httpx.Response, step: str) -> None:
    if resp.status_code >= 400:
        raise AuthError(
            f"[{step}] HTTP {resp.status_code} from {resp.url}\n{resp.text[:400]}"
        )


def login(
    username: str,
    password: str,
    bypass_code: str,
) -> dict[str, str]:
    """
    Authenticate with USC Brightspace via SAML SSO + Duo bypass code.

    Returns a dict of session tokens/cookies:
        d2lSessionVal, d2lSecureSessionVal, XSRF.Token, access_token, user_id
    """
    client = httpx.Client(
        follow_redirects=False,
        timeout=30.0,
        headers={
            "User-Agent": _USER_AGENT,
            "Accept": (
                "text/html,application/xhtml+xml,application/xml;"
                "q=0.9,image/avif,image/webp,*/*;q=0.8"
            ),
            "Accept-Language": "en-US,en;q=0.9",
        },
    )

    try:
        return _do_login(client, username, password, bypass_code)
    finally:
        client.close()


# ---------------------------------------------------------------------------
# Internal helpers
# ---------------------------------------------------------------------------

def _follow(client: httpx.Client, resp: httpx.Response) -> httpx.Response:
    """Manually follow a single redirect, preserving cookies."""
    loc = resp.headers.get("location", "")
    if not loc:
        raise AuthError(f"Expected redirect but got no Location from {resp.url}")
    # Resolve relative URLs against the original request URL
    next_url = str(httpx.URL(loc) if loc.startswith("http") else resp.url.copy_with(path=loc))
    next_resp = client.get(next_url)
    return next_resp


def _extract_xsrf(client: httpx.Client) -> str:
    """Return the _xsrf value from the Duo cookie jar."""
    for cookie in client.cookies.jar:
        if cookie.name == "_xsrf" and DUO_HOST in (cookie.domain or ""):
            return cookie.value
    # Try _xsrf without domain check
    try:
        return client.cookies[f"https://{DUO_HOST}"]["_xsrf"]
    except Exception:
        pass
    # Fallback: iterate all
    for cookie in client.cookies.jar:
        if cookie.name == "_xsrf":
            return cookie.value
    raise AuthError("Could not find Duo _xsrf cookie after loading frameless auth page")


def _do_login(
    client: httpx.Client,
    username: str,
    password: str,
    bypass_code: str,
) -> dict[str, str]:

    # -----------------------------------------------------------------------
    # Step 1-3: Initiate SAML flow, get login page URL
    # -----------------------------------------------------------------------
    resp = client.get(f"https://{BRIGHTSPACE_HOST}/d2l/home")
    # May get 200 if already logged in, or 302 to login
    if resp.status_code == 200 and "/d2l/home" in str(resp.url):
        raise AuthError(
            "Got 200 on /d2l/home without auth — "
            "session may already exist or something is wrong"
        )

    # Follow redirects manually so we can inspect each step
    # Brightspace → /d2l/login → /d2l/lp/auth/saml/initiate-login
    if resp.status_code in (302, 303):
        resp = _follow(client, resp)
    # → login.usc.edu SSO redirect (may be 302 again)
    if resp.status_code in (302, 303):
        resp = _follow(client, resp)
    # Now at login.usc.edu SSO page — follow once more to the login form
    if resp.status_code in (302, 303):
        resp = _follow(client, resp)

    _raise_for(resp, "saml-initiation")

    # Parse the login page URL to extract `goto` param (needed for POST)
    login_page_url = str(resp.url)
    # (goto and service params extracted here if needed for future use)

    # -----------------------------------------------------------------------
    # Step 4: POST username + password
    # -----------------------------------------------------------------------
    post_url = f"https://{LOGIN_HOST}/login/authuserpassword"
    resp = client.post(
        post_url,
        data={
            "j_username": username,
            "j_password": password,
            "_eventId_proceed": "",
        },
        headers={
            "Content-Type": "application/x-www-form-urlencoded",
            "Origin": f"https://{LOGIN_HOST}",
            "Referer": login_page_url,
        },
    )
    # Expect 302 → Duo OAuth authorize URL
    if resp.status_code not in (302, 303):
        raise AuthError(
            f"[auth-userpassword] Expected redirect, got {resp.status_code}. "
            "Credentials may be wrong."
        )

    duo_authorize_url = resp.headers["location"]
    if "duosecurity.com" not in duo_authorize_url:
        raise AuthError(
            f"[auth-userpassword] Expected redirect to Duo, got: {duo_authorize_url!r}"
        )

    # -----------------------------------------------------------------------
    # Step 5: GET Duo OAuth authorize → 303 → frameless/v4/auth
    # -----------------------------------------------------------------------
    resp = client.get(duo_authorize_url, headers={"Referer": f"https://{LOGIN_HOST}/"})
    if resp.status_code not in (302, 303):
        raise AuthError(
            f"[duo-authorize] Expected redirect, got {resp.status_code}"
        )

    frameless_url = f"https://{DUO_HOST}{resp.headers['location']}"
    # Extract sid and tx from URL
    parsed_fl = urllib.parse.urlparse(frameless_url)
    fl_qs = urllib.parse.parse_qs(parsed_fl.query)
    sid = fl_qs.get("sid", [""])[0]
    tx = fl_qs.get("tx", [""])[0]

    if not sid or not tx:
        raise AuthError(f"[duo-authorize] Could not extract sid/tx from {frameless_url!r}")

    # -----------------------------------------------------------------------
    # Step 6: GET frameless/v4/auth — loads Duo frame, sets _xsrf cookie
    # -----------------------------------------------------------------------
    resp = client.get(
        frameless_url,
        headers={"Referer": f"https://{LOGIN_HOST}/"},
    )
    _raise_for(resp, "duo-frameless-get")

    # _xsrf cookie is set by this page
    xsrf = _extract_xsrf(client)

    # -----------------------------------------------------------------------
    # Step 7: POST frameless/v4/auth with tx + _xsrf → 303 → healthcheck
    # -----------------------------------------------------------------------
    resp = client.post(
        frameless_url,
        data={
            "tx": tx,
            "parent": "None",
            "_xsrf": xsrf,
            "version": "v4",
            "akey": "",
            "has_session_trust_analysis_feature": "False",
            "session_trust_extension_id": "",
            "java_version": "",
            "flash_version": "",
            "screen_resolution_width": "1920",
            "screen_resolution_height": "1080",
            "extension_instance_key": "",
            "color_depth": "24",
            "has_touch_capability": "false",
            "ch_ua_error": "",
            "client_hints": "",
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
            "Origin": f"https://{DUO_HOST}",
            "Referer": frameless_url,
            "Content-Type": "application/x-www-form-urlencoded",
        },
    )
    if resp.status_code not in (302, 303):
        raise AuthError(
            f"[duo-frameless-post] Expected redirect, got {resp.status_code}: {resp.text[:200]}"
        )

    # Follow to healthcheck/preauth
    healthcheck_path = resp.headers["location"]
    healthcheck_url = (
        healthcheck_path
        if healthcheck_path.startswith("http")
        else f"https://{DUO_HOST}{healthcheck_path}"
    )
    resp = client.get(healthcheck_url, headers={"Referer": frameless_url})
    # healthcheck might redirect again to auth/prompt
    if resp.status_code in (302, 303):
        loc = resp.headers["location"]
        url = loc if loc.startswith("http") else f"https://{DUO_HOST}{loc}"
        resp = client.get(url, headers={"Referer": healthcheck_url})

    # -----------------------------------------------------------------------
    # Step 8: POST frameless/v4/auth again → 302 → /frame/v4/auth/prompt
    # -----------------------------------------------------------------------
    # After healthcheck, we need to POST frameless again with tx to reach auth prompt
    resp2 = client.post(
        frameless_url,
        data={
            "tx": tx,
            "parent": "None",
            "_xsrf": xsrf,
            "version": "v4",
            "akey": "",
            "has_session_trust_analysis_feature": "False",
            "session_trust_extension_id": "",
            "java_version": "",
            "flash_version": "",
            "screen_resolution_width": "1920",
            "screen_resolution_height": "1080",
            "extension_instance_key": "",
            "color_depth": "24",
            "has_touch_capability": "false",
            "ch_ua_error": "",
            "client_hints": "",
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
            "Origin": f"https://{DUO_HOST}",
            "Referer": frameless_url,
            "Content-Type": "application/x-www-form-urlencoded",
        },
    )
    if resp2.status_code in (302, 303):
        auth_prompt_path = resp2.headers["location"]
        auth_prompt_url = (
            auth_prompt_path
            if auth_prompt_path.startswith("http")
            else f"https://{DUO_HOST}{auth_prompt_path}"
        )
        # GET the auth prompt page (loads React app)
        client.get(auth_prompt_url, headers={"Referer": frameless_url})

    # -----------------------------------------------------------------------
    # Step 9: POST /frame/v4/prompt with Passcode (bypass code)
    # -----------------------------------------------------------------------
    prompt_url = f"https://{DUO_HOST}/frame/v4/prompt"
    prompt_data = {
        "passcode": bypass_code,
        "device": "null",
        "factor": "Passcode",
        "postAuthDestination": "OIDC_EXIT",
        "browser_features": _BROWSER_FEATURES,
        "sid": sid,
    }
    resp = client.post(
        prompt_url,
        data=prompt_data,
        headers={
            "Origin": f"https://{DUO_HOST}",
            "Referer": f"https://{DUO_HOST}/frame/v4/auth/prompt?sid={sid}",
            "X-Xsrftoken": xsrf,
            "Accept": "application/json, text/javascript, */*; q=0.01",
            "Content-Type": "application/x-www-form-urlencoded; charset=UTF-8",
            "X-Requested-With": "XMLHttpRequest",
        },
    )
    _raise_for(resp, "duo-prompt")
    prompt_json = resp.json()
    txid = prompt_json.get("response", {}).get("txid") or prompt_json.get("txid")
    if not txid:
        raise AuthError(
            f"[duo-prompt] Could not get txid from response: {prompt_json}"
        )

    # -----------------------------------------------------------------------
    # Step 10: POST /frame/v4/status — poll until "allow"
    # -----------------------------------------------------------------------
    status_url = f"https://{DUO_HOST}/frame/v4/status"
    status_referer = f"https://{DUO_HOST}/frame/v4/auth/prompt?sid={sid}"

    for attempt in range(10):
        resp = client.post(
            status_url,
            data={"txid": txid, "sid": sid},
            headers={
                "Origin": f"https://{DUO_HOST}",
                "Referer": status_referer,
                "X-Xsrftoken": xsrf,
                "Accept": "application/json, text/javascript, */*; q=0.01",
                "Content-Type": "application/x-www-form-urlencoded; charset=UTF-8",
                "X-Requested-With": "XMLHttpRequest",
            },
        )
        _raise_for(resp, "duo-status")
        status_json = resp.json()
        stat = (
            status_json.get("response", {}).get("result")
            or status_json.get("stat")
            or ""
        )
        result_code = status_json.get("response", {}).get("result", "")
        if result_code == "SUCCESS" or stat == "OK":
            break
        if result_code in ("DENY", "FAILURE"):
            raise AuthError(
                f"[duo-status] Duo denied auth: {status_json}"
            )
        # Still pending — wait and retry
        time.sleep(1.5)
    else:
        raise AuthError("[duo-status] Timed out waiting for Duo approval")

    # -----------------------------------------------------------------------
    # Step 11: POST /frame/v4/oidc/exit → redirect to login.usc.edu/authduo
    # -----------------------------------------------------------------------
    exit_url = f"https://{DUO_HOST}/frame/v4/oidc/exit"
    resp = client.post(
        exit_url,
        data={
            "sid": sid,
            "txid": txid,
            "factor": "Bypass Code",
            "device_key": "",
            "_xsrf": xsrf,
            "dampen_choice": "true",
        },
        headers={
            "Origin": f"https://{DUO_HOST}",
            "Referer": status_referer,
            "Content-Type": "application/x-www-form-urlencoded",
        },
    )
    if resp.status_code not in (302, 303):
        raise AuthError(
            f"[duo-oidc-exit] Expected redirect, got {resp.status_code}: {resp.text[:200]}"
        )

    authduo_url = resp.headers["location"]
    if not authduo_url.startswith("http"):
        authduo_url = f"https://{LOGIN_HOST}{authduo_url}"

    # -----------------------------------------------------------------------
    # Step 12: GET /login/authduo?state=...&duo_code=... → 302 → SAML continue
    # -----------------------------------------------------------------------
    resp = client.get(
        authduo_url,
        headers={"Referer": f"https://{DUO_HOST}/"},
    )
    if resp.status_code in (302, 303):
        saml_continue_url = resp.headers["location"]
        if not saml_continue_url.startswith("http"):
            saml_continue_url = f"https://{LOGIN_HOST}{saml_continue_url}"
    else:
        _raise_for(resp, "authduo")
        # Might be a direct page load
        saml_continue_url = str(resp.url)

    # -----------------------------------------------------------------------
    # Step 13: GET SAML continue page → extract SAMLResponse + RelayState
    # -----------------------------------------------------------------------
    resp = client.get(
        saml_continue_url,
        headers={"Referer": authduo_url},
    )
    _raise_for(resp, "saml-continue")

    # Page may still 302 to the SSO SSORedirect endpoint
    if resp.status_code in (302, 303):
        next_url = resp.headers["location"]
        if not next_url.startswith("http"):
            next_url = f"https://{LOGIN_HOST}{next_url}"
        resp = client.get(next_url, headers={"Referer": saml_continue_url})
        _raise_for(resp, "saml-redirect")
        saml_continue_url = str(resp.url)

    # The page should now contain a form that auto-POSTs SAMLResponse
    # We may need to follow additional intermediate pages
    # Check if we got an HTML page with a SAMLResponse form
    saml_html = resp.text if resp.status_code == 200 else ""
    saml_response, relay_state, saml_post_url = _extract_saml_form(saml_html)

    if not saml_response:
        # Try POSTing the saml2Request form (intermediate page)
        saml2_request = _extract_input(saml_html, "saml2Request")
        if saml2_request:
            # This is the login.usc.edu → Brightspace intermediate POST
            # Find the form action
            form_action = _extract_form_action(saml_html) or saml_continue_url
            resp = client.post(
                form_action if form_action.startswith("http") else f"https://{LOGIN_HOST}{form_action}",
                data={"saml2Request": saml2_request},
                headers={
                    "Content-Type": "application/x-www-form-urlencoded",
                    "Origin": f"https://{LOGIN_HOST}",
                    "Referer": saml_continue_url,
                },
            )
            saml_html = resp.text if resp.status_code == 200 else ""
            saml_response, relay_state, saml_post_url = _extract_saml_form(saml_html)

    if not saml_response:
        raise AuthError(
            "[saml-form] Could not extract SAMLResponse from HTML. "
            f"URL was: {resp.url}, Status: {resp.status_code}"
        )

    # -----------------------------------------------------------------------
    # Step 14: POST SAMLResponse to Brightspace → session established
    # -----------------------------------------------------------------------
    if not saml_post_url.startswith("http"):
        saml_post_url = f"https://{BRIGHTSPACE_HOST}{saml_post_url}"

    post_data: dict[str, str] = {"SAMLResponse": saml_response}
    if relay_state:
        post_data["RelayState"] = relay_state

    resp = client.post(
        saml_post_url,
        data=post_data,
        headers={
            "Content-Type": "application/x-www-form-urlencoded",
            "Origin": f"https://{LOGIN_HOST}",
            "Referer": str(resp.url),
        },
    )
    # Expect redirect to /d2l/home
    if resp.status_code not in (200, 302, 303):
        raise AuthError(
            f"[saml-post] Expected success/redirect, got {resp.status_code}"
        )

    # -----------------------------------------------------------------------
    # Extract session cookies from jar
    # -----------------------------------------------------------------------
    cookies = {}
    for cookie in client.cookies.jar:
        if BRIGHTSPACE_HOST in (cookie.domain or ""):
            cookies[cookie.name] = cookie.value

    if not cookies.get("d2lSessionVal"):
        raise AuthError(
            "[session] d2lSessionVal cookie not found after SAML POST. "
            f"Cookies present: {list(cookies.keys())}"
        )

    return {
        "d2lSessionVal": cookies.get("d2lSessionVal", ""),
        "d2lSecureSessionVal": cookies.get("d2lSecureSessionVal", ""),
        # XSRF and JWT tokens are set via JavaScript/localStorage;
        # need to be fetched via an authenticated GET to /d2l/home
        # and extracted from page script data. Stub for now.
        "xsrf_token": "",
        "access_token": "",
        "user_id": "",
        "_all_cookies": cookies,
    }


# ---------------------------------------------------------------------------
# HTML parsing helpers (no BeautifulSoup dependency)
# ---------------------------------------------------------------------------

def _extract_input(html: str, name: str) -> str:
    """Extract value of a named hidden input field."""
    m = re.search(
        rf'<input[^>]+name=["\']?{re.escape(name)}["\']?[^>]+value=["\']([^"\']*)["\']',
        html,
        re.IGNORECASE,
    ) or re.search(
        rf'<input[^>]+value=["\']([^"\']*)["\'][^>]+name=["\']?{re.escape(name)}["\']?',
        html,
        re.IGNORECASE,
    )
    return m.group(1) if m else ""


def _extract_form_action(html: str) -> str:
    m = re.search(r'<form[^>]+action=["\']([^"\']+)["\']', html, re.IGNORECASE)
    return m.group(1) if m else ""


def _extract_saml_form(html: str) -> tuple[str, str, str]:
    """Return (SAMLResponse, RelayState, form_action) from an auto-submit SAML form."""
    saml_response = _extract_input(html, "SAMLResponse")
    relay_state = _extract_input(html, "RelayState")
    form_action = _extract_form_action(html)
    return saml_response, relay_state, form_action
