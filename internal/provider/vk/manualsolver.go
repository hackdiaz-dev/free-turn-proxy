package vk

import (
	"context"
	"fmt"
	"net"
	"sync"
	"time"

	"github.com/samosvalishe/free-turn-proxy/internal/provider/vk/internal/browserprofile"
	"github.com/samosvalishe/free-turn-proxy/internal/provider/vk/internal/captcha"
	manualcaptcha "github.com/samosvalishe/free-turn-proxy/internal/provider/vk/internal/captcha/manual"
)

var proxyManualMu sync.Mutex
var proxyPowCooldownMu sync.Mutex
var proxyPowCooldownUntil time.Time

const proxyPowFailureCooldown = 90 * time.Second

// ProxyManualSolver показывает UI после запуска локального captcha-прокси.
func ProxyManualSolver(show func(url string), hide func()) ManualSolverFunc {
	return func(ctx context.Context, e *captcha.Error, dialer net.Dialer, profile browserprofile.Profile) (string, error) {
		if e.RedirectURI == "" {
			return "", fmt.Errorf("manual captcha: no redirect_uri")
		}

		proxyManualMu.Lock()
		defer proxyManualMu.Unlock()
		if hide != nil {
			defer hide()
		}

		return manualcaptcha.SolveViaProxyWithPresenter(ctx, e.RedirectURI, dialer, profile, show)
	}
}

// ProxyPowCapture exposes the in-app browser to the automatic PoW capture flow.
func ProxyPowCapture(show func(url string), hide func()) PowCaptureFunc {
	return func(ctx context.Context, redirectURI string, dialer net.Dialer, profile browserprofile.Profile) (string, error) {
		if redirectURI == "" {
			return "", fmt.Errorf("browser pow capture: no redirect_uri")
		}
		proxyPowCooldownMu.Lock()
		cooldown := time.Until(proxyPowCooldownUntil)
		proxyPowCooldownMu.Unlock()
		if cooldown > 0 {
			return "", fmt.Errorf("browser pow capture cooling down for %s after previous capture failure", cooldown.Round(time.Second))
		}
		// A missing background WebView must not queue every concurrent auth stream
		// behind a 12-second wait. Let this request fall back to the legacy solver.
		if !proxyManualMu.TryLock() {
			return "", fmt.Errorf("browser pow capture already in progress")
		}
		defer proxyManualMu.Unlock()
		if hide != nil {
			defer hide()
		}
		pow, err := manualcaptcha.CapturePowViaProxyWithPresenter(ctx, redirectURI, dialer, profile, show)
		proxyPowCooldownMu.Lock()
		if err != nil {
			proxyPowCooldownUntil = time.Now().Add(proxyPowFailureCooldown)
		} else {
			proxyPowCooldownUntil = time.Time{}
		}
		proxyPowCooldownMu.Unlock()
		return pow, err
	}
}
