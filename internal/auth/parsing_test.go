package auth

import (
	"net/url"
	"testing"
)

func TestMicrosoftKMSIFormUsesCurrentPageState(t *testing.T) {
	pageURL, err := url.Parse("https://login.microsoftonline.com/kmsi")
	if err != nil {
		t.Fatal(err)
	}
	page := &page{
		URL: pageURL,
		Body: []byte(`
			<script>
			$Config={"urlPost":"/kmsi","sCtx":"new-context","sFT":"new-token","canary":"new-canary","sessionId":"new-request"};
			</script>`),
	}
	form, ok, err := microsoftForm(page, map[string]string{
		"urlPost":   "/appverify",
		"sCtx":      "stale-context",
		"sFT":       "stale-token",
		"canary":    "stale-canary",
		"sessionId": "stale-request",
	})
	if err != nil {
		t.Fatal(err)
	}
	if !ok {
		t.Fatal("expected a Microsoft form")
	}
	if form.Action != "/kmsi" {
		t.Fatalf("action = %q, want /kmsi", form.Action)
	}
	want := map[string]string{
		"ctx": "new-context", "flowToken": "new-token",
		"canary": "new-canary", "hpgrequestid": "new-request",
	}
	for _, field := range form.Fields {
		if expected, exists := want[field.Name]; exists {
			if field.Value != expected {
				t.Errorf("%s = %q, want %q", field.Name, field.Value, expected)
			}
			delete(want, field.Name)
		}
	}
	if len(want) != 0 {
		t.Fatalf("missing fields: %v", want)
	}
}

func TestMicrosoftFormRejectsIncompleteRecognizedTransition(t *testing.T) {
	pageURL, err := url.Parse("https://login.microsoftonline.com/kmsi")
	if err != nil {
		t.Fatal(err)
	}
	page := &page{
		URL:  pageURL,
		Body: []byte(`<script>$Config={"urlPost":"/kmsi","sCtx":"context"};</script>`),
	}
	if _, _, err := microsoftForm(page, nil); err == nil {
		t.Fatal("expected missing state to be rejected")
	}
}

func TestAutoFormPreservesFieldOrder(t *testing.T) {
	pageURL, err := url.Parse("https://login.usc.edu/login/login")
	if err != nil {
		t.Fatal(err)
	}
	page := &page{URL: pageURL, Body: []byte(`
		<form method="post" action="/continue">
			<input name="first" value="one">
			<input name="second" value="two">
		</form>`)}
	form, ok := autoForm(page)
	if !ok {
		t.Fatal("expected a form")
	}
	if got := encodeFields(form.Fields); got != "first=one&second=two" {
		t.Fatalf("encoded fields = %q", got)
	}
}

func TestHTMLNavigationUsesParsedMetaAndScriptElements(t *testing.T) {
	pageURL, err := url.Parse("https://login.usc.edu/start")
	if err != nil {
		t.Fatal(err)
	}
	page := &page{URL: pageURL, Body: []byte(`
		<div data-state="window.location='https://untrusted.example/'"></div>
		<script>window.location.replace('/continue')</script>`)}
	destination, ok := htmlNavigation(page)
	if !ok {
		t.Fatal("expected script navigation")
	}
	if got, want := destination.String(), "https://login.usc.edu/continue"; got != want {
		t.Fatalf("destination = %q, want %q", got, want)
	}

	page.Body = []byte(`<meta http-equiv="refresh" content="0; url=/refreshed">`)
	destination, ok = htmlNavigation(page)
	if !ok {
		t.Fatal("expected meta refresh navigation")
	}
	if got, want := destination.String(), "https://login.usc.edu/refreshed"; got != want {
		t.Fatalf("destination = %q, want %q", got, want)
	}
}

func TestMicrosoftValuesOnlyReadScriptElements(t *testing.T) {
	values := microsoftValues([]byte(`
		<div data-state='$Config={"urlPost":"/kmsi","sCtx":"decoy"}'></div>
		<script>$Config={"urlPost":"/kmsi","sCtx":"actual"};</script>`))
	if got, want := values["sCtx"], "actual"; got != want {
		t.Fatalf("sCtx = %q, want %q", got, want)
	}
}

func TestCredentialsComplete(t *testing.T) {
	complete := Credentials{Username: "student", Password: "secret", BypassCode: "code"}
	if !complete.Complete() {
		t.Fatal("complete credentials reported incomplete")
	}
	complete.Password = ""
	if complete.Complete() {
		t.Fatal("credentials without a password reported complete")
	}
}
