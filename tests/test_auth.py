"""Tests for USC SSO + Duo bypass code auth flow.

Uses pytest-httpx to intercept all outbound HTTP calls — no real network needed.
The fixtures replicate the exact redirect chain observed in the HAR capture.
"""

from __future__ import annotations

import pytest
import httpx
from pytest_httpx import HTTPXMock

from usc_cli.auth import AuthError, USCAuth


# ---------------------------------------------------------------------------
# Constants matching the HAR
# ---------------------------------------------------------------------------

BRIGHTSPACE = "https://brightspace.usc.edu"
LOGIN = "https://login.usc.edu"
DUO_HOST = "https://api-22627695.duosecurity.com"

SID = "frameless-abc123"
TX = "eyJhbGciOiJIUzUxMiJ9.fake_tx_token.sig"
XSRF = "deadbeef1234567890abcdef"
TXID = "86c1ffc2-b623-4e02-a566-ee2d822ddb79"
DUO_CODE = "aBgOlDDcCwgjrDYxyjyTngduXvPFfobe"
STATE = "795e9b4adfc76f42e69ed80e34124703b015"

SAML_RESPONSE = "PHNhbWxwOlJlc3BvbnNlPmZha2U8L3NhbWxwOlJlc3BvbnNlPg=="  # b64 placeholder

LOGIN_PAGE_HTML = f"""<html><body>
<form method="post" action="{LOGIN}/login/authuserpassword">
  <input name="j_username" />
  <input name="j_password" />
  <input type="submit" />
</form></body></html>"""

SAML_FORM_HTML = f"""<html><body>
<form method="post" action="{BRIGHTSPACE}/d2l/lp/auth/login/samlLogin.d2l">
  <input type="hidden" name="SAMLResponse" value="{SAML_RESPONSE}" />
  <input type="submit" value="Submit" />
</form></body></html>"""


# ---------------------------------------------------------------------------
# Helpers
# ---------------------------------------------------------------------------

def _make_client() -> httpx.Client:
    return httpx.Client(follow_redirects=False, timeout=10.0)


def _register_full_happy_path(httpx_mock: HTTPXMock) -> None:
    """Register all mocked responses for a successful login."""

    # 1. GET /d2l/home → 302 to /d2l/login
    httpx_mock.add_response(
        method="GET",
        url=f"{BRIGHTSPACE}/d2l/home",
        status_code=302,
        headers={"location": f"{BRIGHTSPACE}/d2l/login?sessionExpired=0&target=%2fd2l%2fhome"},
    )

    # 2. GET /d2l/login → 302 to /d2l/lp/auth/saml/initiate-login
    httpx_mock.add_response(
        method="GET",
        url=f"{BRIGHTSPACE}/d2l/login?sessionExpired=0&target=%2fd2l%2fhome",
        status_code=302,
        headers={
            "location": (
                f"{BRIGHTSPACE}/d2l/lp/auth/saml/initiate-login"
                "?entityId=https%3a%2f%2flogin.usc.edu%2fsso&target=%2fd2l%2fhome"
            )
        },
    )

    # 3. GET saml/initiate-login → 302 to login.usc.edu SSO
    httpx_mock.add_response(
        method="GET",
        url=(
            f"{BRIGHTSPACE}/d2l/lp/auth/saml/initiate-login"
            "?entityId=https%3a%2f%2flogin.usc.edu%2fsso&target=%2fd2l%2fhome"
        ),
        status_code=302,
        headers={"location": f"{LOGIN}/sso/SSORedirect/metaAlias/USCRealm/idp?SAMLRequest=fake"},
    )

    # 4. GET SSO redirect → 200 (redirects internally to login page)
    httpx_mock.add_response(
        method="GET",
        url=f"{LOGIN}/sso/SSORedirect/metaAlias/USCRealm/idp?SAMLRequest=fake",
        status_code=302,
        headers={
            "location": (
                f"{LOGIN}/login/login?spEntityID=https://2c451d9d.tenants.brightspace.com"
                "/samlLogin&service=login&goto=https://login.usc.edu:443/sso/saml2/continue"
            )
        },
    )

    # 5. GET login page → 200
    httpx_mock.add_response(
        method="GET",
        url=(
            f"{LOGIN}/login/login?spEntityID=https://2c451d9d.tenants.brightspace.com"
            "/samlLogin&service=login&goto=https://login.usc.edu:443/sso/saml2/continue"
        ),
        status_code=200,
        text=LOGIN_PAGE_HTML,
    )

    # 6. POST authuserpassword → 302 to Duo OAuth
    duo_oauth_url = (
        f"{DUO_HOST}/oauth/v1/authorize?scope=openid&response_type=code"
        f"&redirect_uri={LOGIN}/login/authduo&client_id=DI1TG76X&request={TX}"
    )
    httpx_mock.add_response(
        method="POST",
        url=f"{LOGIN}/login/authuserpassword",
        status_code=302,
        headers={"location": duo_oauth_url},
    )

    # 7. GET Duo OAuth → 303 to frameless
    frameless_url = f"{DUO_HOST}/frame/frameless/v4/auth?sid={SID}&tx={TX}"
    httpx_mock.add_response(
        method="GET",
        url=duo_oauth_url,
        status_code=303,
        headers={"location": f"/frame/frameless/v4/auth?sid={SID}&tx={TX}"},
    )

    # 8. GET frameless page → 200 (sets _xsrf cookie)
    httpx_mock.add_response(
        method="GET",
        url=frameless_url,
        status_code=200,
        headers={"set-cookie": f"_xsrf={XSRF}; HttpOnly; SameSite=Strict"},
        text="<html>duo frameless</html>",
    )

    # 9. POST frameless init → 303 to preauth/healthcheck
    httpx_mock.add_response(
        method="POST",
        url=frameless_url,
        status_code=303,
        headers={"location": f"/frame/v4/preauth/healthcheck?sid={SID}"},
    )

    # 10. GET preauth/healthcheck → 200
    httpx_mock.add_response(
        method="GET",
        url=f"{DUO_HOST}/frame/v4/preauth/healthcheck?sid={SID}",
        status_code=200,
        text="<html>healthcheck</html>",
    )

    # 11. GET return → 303 → frameless → 302 → auth/prompt
    httpx_mock.add_response(
        method="GET",
        url=f"{DUO_HOST}/frame/v4/return?sid={SID}",
        status_code=303,
        headers={"location": f"/frame/frameless/v4/auth?sid={SID}&tx={TX}"},
    )
    httpx_mock.add_response(
        method="GET",
        url=frameless_url,
        status_code=302,
        headers={"location": f"/frame/v4/auth/prompt?sid={SID}"},
    )
    httpx_mock.add_response(
        method="GET",
        url=f"{DUO_HOST}/frame/v4/auth/prompt?sid={SID}",
        status_code=200,
        text="<html>auth prompt</html>",
    )

    # 12. POST /frame/v4/prompt (bypass code) → 200 with txid
    httpx_mock.add_response(
        method="POST",
        url=f"{DUO_HOST}/frame/v4/prompt",
        status_code=200,
        json={"stat": "OK", "response": {"txid": TXID}},
    )

    # 13. POST /frame/v4/status → 200 allow
    httpx_mock.add_response(
        method="POST",
        url=f"{DUO_HOST}/frame/v4/status",
        status_code=200,
        json={"stat": "OK", "response": {"result": "SUCCESS", "status_code": "allow"}},
    )

    # 14. POST /frame/v4/oidc/exit → 303 to authduo with duo_code
    httpx_mock.add_response(
        method="POST",
        url=f"{DUO_HOST}/frame/v4/oidc/exit",
        status_code=303,
        headers={
            "location": f"{LOGIN}/login/authduo?state={STATE}&duo_code={DUO_CODE}"
        },
    )

    # 15. GET authduo → 302 → saml2/continue
    httpx_mock.add_response(
        method="GET",
        url=f"{LOGIN}/login/authduo?state={STATE}&duo_code={DUO_CODE}",
        status_code=302,
        headers={"location": f"{LOGIN}/sso/saml2/continue/metaAlias/USCRealm/idp?secondVisitUrl=/"},
    )

    # 16. GET saml2/continue → 200 with SAMLResponse form
    httpx_mock.add_response(
        method="GET",
        url=f"{LOGIN}/sso/saml2/continue/metaAlias/USCRealm/idp?secondVisitUrl=/",
        status_code=200,
        text=SAML_FORM_HTML,
    )

    # 17. POST samlLogin.d2l → 302 → /d2l/home
    httpx_mock.add_response(
        method="POST",
        url=f"{BRIGHTSPACE}/d2l/lp/auth/login/samlLogin.d2l",
        status_code=302,
        headers={"location": f"{BRIGHTSPACE}/d2l/home"},
    )

    # 18. GET /d2l/home (final landing) → 200
    httpx_mock.add_response(
        method="GET",
        url=f"{BRIGHTSPACE}/d2l/home",
        status_code=200,
        text="<html>home</html>",
    )


# ---------------------------------------------------------------------------
# Unit tests — individual methods
# ---------------------------------------------------------------------------


class TestPostCredentials:
    def test_success_returns_duo_url(self, httpx_mock: HTTPXMock) -> None:
        duo_url = f"{DUO_HOST}/oauth/v1/authorize?client_id=ABC&request={TX}"
        httpx_mock.add_response(
            method="POST",
            url=f"{LOGIN}/login/authuserpassword",
            status_code=302,
            headers={"location": duo_url},
        )
        client = _make_client()
        auth = USCAuth(client)
        result = auth._post_credentials(
            f"{LOGIN}/login/login?spEntityID=x",
            "nsigel",
            "hunter2",
        )
        assert result == duo_url

    def test_bad_password_raises(self, httpx_mock: HTTPXMock) -> None:
        httpx_mock.add_response(
            method="POST",
            url=f"{LOGIN}/login/authuserpassword",
            status_code=200,
            text="<html>Your password is incorrect</html>",
        )
        client = _make_client()
        auth = USCAuth(client)
        with pytest.raises(AuthError, match="Invalid username or password"):
            auth._post_credentials(f"{LOGIN}/login/login", "nsigel", "wrong")

    def test_unexpected_status_raises(self, httpx_mock: HTTPXMock) -> None:
        httpx_mock.add_response(
            method="POST",
            url=f"{LOGIN}/login/authuserpassword",
            status_code=200,
            text="<html>some other page</html>",
        )
        client = _make_client()
        auth = USCAuth(client)
        with pytest.raises(AuthError, match="Expected redirect"):
            auth._post_credentials(f"{LOGIN}/login/login", "nsigel", "pw")

    def test_non_duo_redirect_raises(self, httpx_mock: HTTPXMock) -> None:
        httpx_mock.add_response(
            method="POST",
            url=f"{LOGIN}/login/authuserpassword",
            status_code=302,
            headers={"location": "https://example.com/surprise"},
        )
        client = _make_client()
        auth = USCAuth(client)
        with pytest.raises(AuthError, match="Expected Duo redirect"):
            auth._post_credentials(f"{LOGIN}/login/login", "nsigel", "pw")


class TestInitDuoSession:
    def test_extracts_sid_and_tx(self, httpx_mock: HTTPXMock) -> None:
        duo_oauth_url = (
            f"{DUO_HOST}/oauth/v1/authorize?scope=openid&client_id=ABC&request={TX}"
        )
        frameless = f"/frame/frameless/v4/auth?sid={SID}&tx={TX}"
        httpx_mock.add_response(
            method="GET",
            url=duo_oauth_url,
            status_code=303,
            headers={"location": frameless},
        )
        client = _make_client()
        auth = USCAuth(client)
        host, sid, tx = auth._init_duo_session(duo_oauth_url)
        assert host == "api-22627695.duosecurity.com"
        assert sid == SID
        assert tx == TX

    def test_missing_sid_raises(self, httpx_mock: HTTPXMock) -> None:
        duo_oauth_url = f"{DUO_HOST}/oauth/v1/authorize?client_id=ABC"
        httpx_mock.add_response(
            method="GET",
            url=duo_oauth_url,
            status_code=303,
            headers={"location": "/frame/frameless/v4/auth?nothin=here"},
        )
        client = _make_client()
        auth = USCAuth(client)
        with pytest.raises(AuthError, match="Could not extract sid/tx"):
            auth._init_duo_session(duo_oauth_url)


class TestGetDuoXsrf:
    def test_extracts_xsrf_from_cookie(self, httpx_mock: HTTPXMock) -> None:
        url = f"{DUO_HOST}/frame/frameless/v4/auth"
        httpx_mock.add_response(
            method="GET",
            url=f"{url}?sid={SID}&tx={TX}",
            status_code=200,
            headers={"set-cookie": f"_xsrf={XSRF}; Path=/; HttpOnly"},
            text="<html>duo</html>",
        )
        client = _make_client()
        auth = USCAuth(client)
        xsrf = auth._get_duo_xsrf("api-22627695.duosecurity.com", SID, TX)
        assert xsrf == XSRF

    def test_extracts_xsrf_from_html(self, httpx_mock: HTTPXMock) -> None:
        url = f"{DUO_HOST}/frame/frameless/v4/auth"
        httpx_mock.add_response(
            method="GET",
            url=f"{url}?sid={SID}&tx={TX}",
            status_code=200,
            text=f'<input name="_xsrf" value="{XSRF}" />',
        )
        client = _make_client()
        auth = USCAuth(client)
        xsrf = auth._get_duo_xsrf("api-22627695.duosecurity.com", SID, TX)
        assert xsrf == XSRF

    def test_missing_xsrf_raises(self, httpx_mock: HTTPXMock) -> None:
        url = f"{DUO_HOST}/frame/frameless/v4/auth"
        httpx_mock.add_response(
            method="GET",
            url=f"{url}?sid={SID}&tx={TX}",
            status_code=200,
            text="<html>no xsrf here</html>",
        )
        client = _make_client()
        auth = USCAuth(client)
        with pytest.raises(AuthError, match="Could not extract _xsrf"):
            auth._get_duo_xsrf("api-22627695.duosecurity.com", SID, TX)


class TestPostDuoPrompt:
    def test_success_returns_txid(self, httpx_mock: HTTPXMock) -> None:
        httpx_mock.add_response(
            method="POST",
            url=f"{DUO_HOST}/frame/v4/prompt",
            status_code=200,
            json={"stat": "OK", "response": {"txid": TXID}},
        )
        client = _make_client()
        auth = USCAuth(client)
        txid = auth._post_duo_prompt("api-22627695.duosecurity.com", SID, XSRF, "823114726")
        assert txid == TXID

    def test_rejected_raises(self, httpx_mock: HTTPXMock) -> None:
        httpx_mock.add_response(
            method="POST",
            url=f"{DUO_HOST}/frame/v4/prompt",
            status_code=200,
            json={"stat": "FAIL", "message": "Invalid passcode"},
        )
        client = _make_client()
        auth = USCAuth(client)
        with pytest.raises(AuthError, match="Duo prompt rejected"):
            auth._post_duo_prompt("api-22627695.duosecurity.com", SID, XSRF, "000000")

    def test_missing_txid_raises(self, httpx_mock: HTTPXMock) -> None:
        httpx_mock.add_response(
            method="POST",
            url=f"{DUO_HOST}/frame/v4/prompt",
            status_code=200,
            json={"stat": "OK", "response": {}},
        )
        client = _make_client()
        auth = USCAuth(client)
        with pytest.raises(AuthError, match="No txid"):
            auth._post_duo_prompt("api-22627695.duosecurity.com", SID, XSRF, "111111")


class TestPollDuoStatus:
    def test_allow_returns_immediately(self, httpx_mock: HTTPXMock) -> None:
        httpx_mock.add_response(
            method="POST",
            url=f"{DUO_HOST}/frame/v4/status",
            status_code=200,
            json={"stat": "OK", "response": {"result": "SUCCESS", "status_code": "allow"}},
        )
        client = _make_client()
        auth = USCAuth(client)
        auth._poll_duo_status("api-22627695.duosecurity.com", SID, TXID)  # no exception

    def test_deny_raises(self, httpx_mock: HTTPXMock) -> None:
        httpx_mock.add_response(
            method="POST",
            url=f"{DUO_HOST}/frame/v4/status",
            status_code=200,
            json={"stat": "OK", "response": {"result": "FAILURE", "status_code": "deny", "reason": "Code expired"}},
        )
        client = _make_client()
        auth = USCAuth(client)
        with pytest.raises(AuthError, match="Duo MFA denied"):
            auth._poll_duo_status("api-22627695.duosecurity.com", SID, TXID)

    @pytest.mark.httpx_mock(assert_all_responses_were_requested=False)
    def test_timeout_raises(self, httpx_mock: HTTPXMock) -> None:
        # Register enough responses to cover every poll attempt within max_wait=1s / interval=0.1s
        for _ in range(20):
            httpx_mock.add_response(
                method="POST",
                url=f"{DUO_HOST}/frame/v4/status",
                status_code=200,
                json={"stat": "OK", "response": {"result": "WAITING", "status_code": "pushed"}},
            )
        client = _make_client()
        auth = USCAuth(client)
        with pytest.raises(AuthError, match="timed out"):
            auth._poll_duo_status(
                "api-22627695.duosecurity.com", SID, TXID, max_wait=1, interval=0.1
            )


class TestDuoOidcExit:
    def test_returns_duo_code_and_state(self, httpx_mock: HTTPXMock) -> None:
        redirect = f"{LOGIN}/login/authduo?state={STATE}&duo_code={DUO_CODE}"
        httpx_mock.add_response(
            method="POST",
            url=f"{DUO_HOST}/frame/v4/oidc/exit",
            status_code=303,
            headers={"location": redirect},
        )
        client = _make_client()
        auth = USCAuth(client)
        code, state = auth._duo_oidc_exit("api-22627695.duosecurity.com", SID, TXID, XSRF)
        assert code == DUO_CODE
        assert state == STATE

    def test_missing_duo_code_raises(self, httpx_mock: HTTPXMock) -> None:
        httpx_mock.add_response(
            method="POST",
            url=f"{DUO_HOST}/frame/v4/oidc/exit",
            status_code=303,
            headers={"location": f"{LOGIN}/login/authduo?state={STATE}"},
        )
        client = _make_client()
        auth = USCAuth(client)
        with pytest.raises(AuthError, match="Missing duo_code/state"):
            auth._duo_oidc_exit("api-22627695.duosecurity.com", SID, TXID, XSRF)


class TestExtractSamlResponse:
    def test_standard_form(self) -> None:
        html = f"""<form action="{BRIGHTSPACE}/d2l/lp/auth/login/samlLogin.d2l">
            <input name="SAMLResponse" value="{SAML_RESPONSE}" />
        </form>"""
        client = _make_client()
        auth = USCAuth(client)
        resp = httpx.Response(200, text=html, request=httpx.Request("GET", f"{LOGIN}/page"))
        saml, url = auth._extract_saml_response(resp)
        assert saml == SAML_RESPONSE
        assert "samlLogin.d2l" in url

    def test_value_before_name(self) -> None:
        html = f"""<form action="{BRIGHTSPACE}/d2l/lp/auth/login/samlLogin.d2l">
            <input value="{SAML_RESPONSE}" name="SAMLResponse" />
        </form>"""
        client = _make_client()
        auth = USCAuth(client)
        resp = httpx.Response(200, text=html, request=httpx.Request("GET", f"{LOGIN}/page"))
        saml, url = auth._extract_saml_response(resp)
        assert saml == SAML_RESPONSE

    def test_missing_saml_response_raises(self) -> None:
        html = "<html><body>no form here</body></html>"
        client = _make_client()
        auth = USCAuth(client)
        resp = httpx.Response(200, text=html, request=httpx.Request("GET", f"{LOGIN}/page"))
        with pytest.raises(AuthError, match="Could not find SAMLResponse"):
            auth._extract_saml_response(resp)

    def test_missing_post_url_raises(self) -> None:
        html = f'<input name="SAMLResponse" value="{SAML_RESPONSE}" />'
        client = _make_client()
        auth = USCAuth(client)
        resp = httpx.Response(200, text=html, request=httpx.Request("GET", f"{LOGIN}/page"))
        with pytest.raises(AuthError, match="Could not find SAML POST URL"):
            auth._extract_saml_response(resp)


class TestPostSaml:
    def test_success(self, httpx_mock: HTTPXMock) -> None:
        httpx_mock.add_response(
            method="POST",
            url=f"{BRIGHTSPACE}/d2l/lp/auth/login/samlLogin.d2l",
            status_code=302,
            headers={"location": f"{BRIGHTSPACE}/d2l/home"},
        )
        httpx_mock.add_response(
            method="GET",
            url=f"{BRIGHTSPACE}/d2l/home",
            status_code=200,
            text="<html>home</html>",
        )
        client = _make_client()
        auth = USCAuth(client)
        auth._post_saml(
            f"{BRIGHTSPACE}/d2l/lp/auth/login/samlLogin.d2l",
            SAML_RESPONSE,
        )

    def test_still_on_auth_page_raises(self, httpx_mock: HTTPXMock) -> None:
        httpx_mock.add_response(
            method="POST",
            url=f"{BRIGHTSPACE}/d2l/lp/auth/login/samlLogin.d2l",
            status_code=302,
            headers={"location": f"{BRIGHTSPACE}/d2l/login?sessionExpired=1"},
        )
        httpx_mock.add_response(
            method="GET",
            url=f"{BRIGHTSPACE}/d2l/login?sessionExpired=1",
            status_code=200,
            text="<html>login</html>",
        )
        client = _make_client()
        auth = USCAuth(client)
        with pytest.raises(AuthError, match="still on auth page"):
            auth._post_saml(
                f"{BRIGHTSPACE}/d2l/lp/auth/login/samlLogin.d2l",
                SAML_RESPONSE,
            )
