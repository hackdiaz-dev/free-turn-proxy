package vk

import (
	"context"
	"errors"
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

		unlock, err := lockCaptchaPage(ctx)
		if err != nil {
			return "", err
		}
		defer unlock()
		if hide != nil {
			defer hide()
		}

		return manualcaptcha.SolveViaProxyWithPresenter(ctx, e.RedirectURI, dialer, profile, show)
	}
}

// lockCaptchaPage занимает локальную страницу капчи (порт 8765 один на процесс). С несколькими
// -links у каждой ссылки свой провайдер и своя капча: без очереди второй провайдер получает
// "bind: address already in use" и засчитывает это как проваленную ручную капчу.
// Пока ждали, сессию могли свернуть или вышел таймаут ручной капчи - тогда страницу не
// поднимаем, иначе пользователю покажется капча, которую уже никто не ждёт.
func lockCaptchaPage(ctx context.Context) (unlock func(), err error) {
	proxyManualMu.Lock()
	if err := ctx.Err(); err != nil {
		proxyManualMu.Unlock()
		return nil, err
	}
	return proxyManualMu.Unlock, nil
}

// ProxyPowCapture exposes the in-app browser to the automatic PoW capture flow.
func ProxyPowCapture(show func(url string), hide func()) PowCaptureFunc {
	return lockedPowCapture(func(ctx context.Context, redirectURI string, dialer net.Dialer, profile browserprofile.Profile) (string, error) {
		if hide != nil {
			defer hide()
		}
		return manualcaptcha.CapturePowViaProxyWithPresenter(ctx, redirectURI, dialer, profile, show)
	})
}

// DefaultPowCapture - захват PoW для CLI: страница открывается в системном браузере, с той же
// очередью и паузой после сбоя, что и у ProxyPowCapture.
func DefaultPowCapture(ctx context.Context, redirectURI string, dialer net.Dialer, profile browserprofile.Profile) (string, error) {
	return lockedPowCapture(manualcaptcha.CapturePowViaProxy)(ctx, redirectURI, dialer, profile)
}

func lockedPowCapture(capture PowCaptureFunc) PowCaptureFunc {
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
		pow, err := capture(ctx, redirectURI, dialer, profile)
		recordPowCaptureResult(err)
		return pow, err
	}
}

// recordPowCaptureResult обновляет паузу после попытки захвата PoW.
// Отмена контекста (перезапуск релея, остановка) не считается сбоем захвата.
func recordPowCaptureResult(err error) {
	proxyPowCooldownMu.Lock()
	defer proxyPowCooldownMu.Unlock()
	if err == nil || errors.Is(err, context.Canceled) {
		proxyPowCooldownUntil = time.Time{}
		return
	}
	proxyPowCooldownUntil = time.Now().Add(proxyPowFailureCooldown)
}
