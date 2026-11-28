package weblogin

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/nsigel/usc-cli/auth"
)

const (
	stepCredentials = "credentials"
	stepWorking     = "working"
	stepMethod      = "method"
	stepCode        = "code"
	stepPush        = "push"
	stepDone        = "done"
	stepFailed      = "failed"
	stepInterrupted = "interrupted"
)

const (
	maxPasswordFailures = 3
	maxDuoFailures      = 5
)

type state struct {
	Step    string           `json:"step"`
	Methods []auth.DuoMethod `json:"methods,omitempty"`
	Prompt  string           `json:"prompt,omitempty"`
	Code    string           `json:"code,omitempty"`
	Round   int              `json:"round,omitempty"`
	Error   string           `json:"error,omitempty"`
}

// flow drives one sign-in. The page moves it forward one step at a time;
// each step that talks to USC runs on its own goroutine.
type flow struct {
	options Options
	ctx     context.Context

	mu               sync.Mutex
	state            state
	credentials      auth.Credentials
	methods          []auth.DuoMethod
	passwordFailures int
	duoFailures      int

	workers   sync.WaitGroup
	codes     chan string
	delivered chan struct{}
	deliver   sync.Once
}

func newFlow(options Options) *flow {
	return &flow{
		options:   options,
		state:     state{Step: stepCredentials},
		codes:     make(chan string, 1),
		delivered: make(chan struct{}),
	}
}

func (f *flow) snapshot() state {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.state
}

func (f *flow) set(next state) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.state = next
}

// begin moves from an input step to working, refusing stale or repeated submissions.
func (f *flow) begin(from string) bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.state.Step != from {
		return false
	}
	f.state = state{Step: stepWorking}
	return true
}

func (f *flow) finish(final state) {
	f.set(final)
	time.AfterFunc(30*time.Second, f.release)
}

// release lets the server stop once the page has seen the final state.
func (f *flow) release() {
	f.deliver.Do(func() { close(f.delivered) })
}

// run starts a step that talks to USC; Serve waits for it before exiting.
func (f *flow) run(step func()) {
	f.workers.Add(1)
	go func() {
		defer f.workers.Done()
		step()
	}()
}

func (f *flow) submitCredentials(username, password string) {
	username = strings.TrimSpace(username)
	if username == "" || password == "" || !f.begin(stepCredentials) {
		return
	}
	if !strings.Contains(username, "@") {
		username += "@usc.edu"
	}
	f.run(func() { f.signIn(auth.Credentials{Username: username, Password: password}) })
}

func (f *flow) signIn(credentials auth.Credentials) {
	saved, err := auth.LoadCredentials(f.options.Auth.CredentialsFile)
	if err != nil {
		f.finish(state{Step: stepFailed, Error: err.Error()})
		return
	}
	if sameAccount(saved.Username, credentials.Username) {
		credentials.DuoDeviceID, credentials.DuoTOTPSecret = saved.DuoDeviceID, saved.DuoTOTPSecret
	}
	if err := os.Remove(f.options.Auth.SessionFile); err != nil && !errors.Is(err, os.ErrNotExist) {
		f.finish(state{Step: stepFailed, Error: err.Error()})
		return
	}
	f.mu.Lock()
	f.credentials = credentials
	f.mu.Unlock()

	if credentials.DuoTOTPSecret == "" {
		f.enroll(f.authOptions())
		return
	}
	if _, err := auth.Open(f.ctx, f.options.Probe, f.authOptions()); err != nil {
		if errors.Is(err, auth.ErrDuoPasscodeRejected) {
			err = errors.New("The saved Duo device no longer works. Sign out, then sign in again.")
		}
		f.retry(err)
		return
	}
	f.save(credentials)
}

func (f *flow) chooseMethod(kind, deviceID, code string) {
	options := f.authOptions()
	switch kind {
	case "push":
		round := 0
		options.DuoPushDevice = deviceID
		options.DuoPushStarted = func(verification string) {
			round++
			f.set(state{Step: stepPush, Code: verification, Round: round})
		}
	case "sms_otp":
		options.DuoSMSDevice = deviceID
		options.RequestDuoPasscode = f.requestCode("sms")
	case "mobile_otp":
		options.RequestDuoPasscode = f.requestCode("mobile")
	case "bypass_code":
		if options.Credentials.BypassCode = strings.TrimSpace(code); options.Credentials.BypassCode == "" {
			return
		}
	default:
		return
	}
	if f.begin(stepMethod) {
		f.run(func() { f.enroll(options) })
	}
}

func (f *flow) requestCode(prompt string) func() (string, error) {
	round := 0
	return func() (string, error) {
		round++
		f.set(state{Step: stepCode, Prompt: prompt, Round: round})
		select {
		case code := <-f.codes:
			return code, nil
		case <-f.ctx.Done():
			return "", f.ctx.Err()
		}
	}
}

func (f *flow) submitCode(code string) {
	code = strings.TrimSpace(code)
	if code != "" && f.begin(stepCode) {
		f.codes <- code
	}
}

// authOptions leaves CredentialsFile empty so nothing is merged from or
// written to disk until the flow saves the finished sign-in itself.
func (f *flow) authOptions() auth.Options {
	f.mu.Lock()
	defer f.mu.Unlock()
	return auth.Options{Credentials: f.credentials, SessionFile: f.options.Auth.SessionFile, DuoDeviceName: f.options.Auth.DuoDeviceName}
}

// enroll signs in and creates this device's Duo authenticator. Without a
// chosen method it stops at the Duo prompt and reports the methods on offer.
func (f *flow) enroll(options auth.Options) {
	device, err := auth.CreateDuoDevice(f.ctx, options)
	switch {
	case device != nil && device.Secret != "":
		credentials := options.Credentials
		credentials.DuoDeviceID, credentials.DuoTOTPSecret, credentials.BypassCode = device.ID, device.Secret, ""
		f.save(credentials)
	case device != nil:
		// A half-created device must be resolved by hand; retrying would add another.
		f.finish(state{Step: stepFailed, Error: err.Error()})
	default:
		f.retry(err)
	}
}

func (f *flow) save(credentials auth.Credentials) {
	if err := auth.SaveCredentials(f.options.Auth.CredentialsFile, credentials); err != nil {
		message := err.Error()
		if credentials.DuoDeviceID != "" {
			message = fmt.Sprintf("Duo device %s was created but could not be saved; remove it in USC device management before retrying: %v", credentials.DuoDeviceID, err)
		}
		f.finish(state{Step: stepFailed, Error: message})
		return
	}
	f.finish(state{Step: stepDone})
}

// retry returns to the step the user can correct, or ends the flow.
func (f *flow) retry(err error) {
	f.mu.Lock()
	var offered *auth.DuoMethodsRequired
	next := state{Step: stepInterrupted, Error: err.Error()}
	switch {
	case errors.As(err, &offered):
		f.methods = devices(offered.Methods)
		next = state{Step: stepMethod, Methods: f.methods}
	case errors.Is(err, auth.ErrPasswordRejected):
		f.passwordFailures++
		next = state{Step: stepCredentials, Error: "Incorrect NetID or password."}
		if f.passwordFailures >= maxPasswordFailures {
			next = state{Step: stepFailed, Error: "Too many incorrect passwords."}
		}
	case errors.Is(err, auth.ErrDuoPushRejected):
		next = f.duoRetry("The push wasn't approved.")
	case errors.Is(err, auth.ErrDuoPasscodeRejected), errors.Is(err, auth.ErrDuoSMSRejected), errors.Is(err, auth.ErrBypassRejected):
		next = f.duoRetry("That code didn't work.")
	case f.ctx.Err() != nil:
		next = state{Step: stepFailed, Error: "This page expired."}
	}
	f.mu.Unlock()
	if next.Step == stepFailed {
		f.finish(next)
		return
	}
	f.set(next)
}

// again reruns a sign-in that an error interrupted, with the credentials
// already entered.
func (f *flow) again() {
	if !f.begin(stepInterrupted) {
		return
	}
	f.mu.Lock()
	credentials := f.credentials
	f.mu.Unlock()
	f.run(func() { f.signIn(credentials) })
}

func (f *flow) duoRetry(message string) state {
	f.duoFailures++
	if f.duoFailures >= maxDuoFailures {
		return state{Step: stepFailed, Error: "Too many failed Duo attempts."}
	}
	return state{Step: stepMethod, Methods: f.methods, Error: message}
}

// devices keeps the methods that need a device chosen; the page always offers
// Duo Mobile and bypass codes.
func devices(methods []auth.DuoMethod) []auth.DuoMethod {
	var chosen []auth.DuoMethod
	for _, method := range methods {
		if method.DeviceID != "" && (method.Type == "push" || method.Type == "sms_otp") {
			chosen = append(chosen, method)
		}
	}
	return chosen
}

func sameAccount(saved, entered string) bool {
	netID := func(name string) string {
		return strings.TrimSuffix(strings.ToLower(strings.TrimSpace(name)), "@usc.edu")
	}
	return saved != "" && netID(saved) == netID(entered)
}
