package vk

import (
	"context"
	"errors"
	"net"
	"testing"
	"time"

	"github.com/samosvalishe/free-turn-proxy/internal/provider/vk/internal/browserprofile"
	"github.com/samosvalishe/free-turn-proxy/internal/provider/vk/internal/captcha"
)

func TestProxyPowCaptureDoesNotWaitBehindManualCaptcha(t *testing.T) {
	proxyManualMu.Lock()
	defer proxyManualMu.Unlock()

	capture := ProxyPowCapture(nil, nil)
	started := time.Now()
	_, err := capture(context.Background(), "http://localhost:8765/not_robot_captcha", net.Dialer{}, browserprofile.Profile{})
	if err == nil {
		t.Fatal("expected busy capture to return an error")
	}
	if elapsed := time.Since(started); elapsed >= time.Second {
		t.Fatalf("busy capture waited %s instead of falling back promptly", elapsed)
	}
}

func TestDefaultPowCaptureDoesNotWaitBehindManualCaptcha(t *testing.T) {
	proxyManualMu.Lock()
	defer proxyManualMu.Unlock()

	started := time.Now()
	_, err := DefaultPowCapture(context.Background(), "http://localhost:8765/not_robot_captcha", net.Dialer{}, browserprofile.Profile{})
	if err == nil {
		t.Fatal("expected busy capture to return an error")
	}
	if elapsed := time.Since(started); elapsed >= time.Second {
		t.Fatalf("busy capture waited %s instead of falling back promptly", elapsed)
	}
}

// С несколькими -links провайдеры делят один порт страницы капчи: второй ждёт первого.
func TestLockCaptchaPageQueuesSecondCaller(t *testing.T) {
	unlockFirst, err := lockCaptchaPage(context.Background())
	if err != nil {
		t.Fatalf("first lock: %v", err)
	}

	acquired := make(chan error, 1)
	go func() {
		unlock, err := lockCaptchaPage(context.Background())
		if err == nil {
			unlock()
		}
		acquired <- err
	}()

	select {
	case err := <-acquired:
		t.Fatalf("second caller got the page while the first held it (err=%v)", err)
	case <-time.After(100 * time.Millisecond):
	}

	unlockFirst()
	select {
	case err := <-acquired:
		if err != nil {
			t.Fatalf("second caller: %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("second caller did not get the page after the first released it")
	}
}

// Ожидание отменили (стоп, таймаут ручной капчи) - страница уже никому не нужна.
func TestLockCaptchaPageSkipsCancelledWaiter(t *testing.T) {
	unlockFirst, err := lockCaptchaPage(context.Background())
	if err != nil {
		t.Fatalf("first lock: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() {
		unlock, err := lockCaptchaPage(ctx)
		if err == nil {
			unlock()
		}
		done <- err
	}()

	cancel()
	unlockFirst()
	select {
	case err := <-done:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("cancelled waiter: got %v, want context.Canceled", err)
		}
	case <-time.After(time.Second):
		t.Fatal("cancelled waiter did not return")
	}

	// Отказавшийся не держит страницу.
	if !proxyManualMu.TryLock() {
		t.Fatal("cancelled waiter left the captcha page locked")
	}
	proxyManualMu.Unlock()
}

func TestDefaultManualSolverDoesNotOpenPageAfterCancel(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := DefaultManualSolver(ctx, &captcha.Error{RedirectURI: "https://id.vk.ru/not_robot_captcha"}, net.Dialer{}, browserprofile.Profile{})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("got %v, want context.Canceled before the page is served", err)
	}
}
