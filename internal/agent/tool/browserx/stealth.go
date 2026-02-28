package browserx

import (
	"github.com/go-rod/rod"
	"github.com/go-rod/rod/lib/launcher"
	"github.com/go-rod/rod/lib/launcher/flags"
	"github.com/go-rod/rod/lib/proto"
	"github.com/go-rod/stealth"
)

// newStealthLauncher creates a launcher with anti-detection Chrome flags.
func newStealthLauncher(headless bool) *launcher.Launcher {
	l := launcher.New()

	if headless {
		// Use new headless mode (Chrome 112+) which is harder to detect.
		l.HeadlessNew(true)
	} else {
		l.Headless(false)
	}

	// Remove automation markers.
	l.Set(flags.Flag("disable-blink-features"), "AutomationControlled")

	// Disable features that leak automation.
	l.Set(flags.Flag("disable-features"), "TranslateUI")
	l.Set(flags.Flag("disable-infobars"))
	l.Set(flags.Flag("disable-dev-shm-usage"))
	l.Set(flags.Flag("no-first-run"))
	l.Set(flags.Flag("no-default-browser-check"))

	// Realistic window size (headless defaults to 800x600 which is a giveaway).
	l.Set(flags.Flag("window-size"), "1920,1080")

	// Language.
	l.Set(flags.Flag("lang"), "en-US")

	// Disable Chrome sandbox in environments that don't support it (CI containers, root).
	if needsNoSandbox() {
		l.Set(flags.Flag("no-sandbox"))
	}

	return l
}

// newStealthPage creates a page using go-rod/stealth and applies extra evasions.
func newStealthPage(browser *rod.Browser) (*rod.Page, error) {
	page, err := stealth.Page(browser)
	if err != nil {
		return nil, err
	}

	if err := applyExtraStealth(page); err != nil {
		page.Close()
		return nil, err
	}

	return page, nil
}

// applyExtraStealth injects additional JS evasions beyond what go-rod/stealth covers.
func applyExtraStealth(page *rod.Page) error {
	js := `
	// Fix window.outerWidth/outerHeight (may be 0 in headless).
	if (window.outerWidth === 0) {
		Object.defineProperty(window, 'outerWidth', { get: () => window.innerWidth });
	}
	if (window.outerHeight === 0) {
		Object.defineProperty(window, 'outerHeight', { get: () => window.innerHeight + 85 });
	}

	// Simulate navigator.connection (NetworkInformation API).
	if (!navigator.connection) {
		Object.defineProperty(navigator, 'connection', {
			get: () => ({
				effectiveType: '4g',
				rtt: 50,
				downlink: 10,
				saveData: false,
			}),
		});
	}

	// Ensure navigator.permissions.query returns consistent results for notifications.
	const originalQuery = window.navigator.permissions.query.bind(window.navigator.permissions);
	window.navigator.permissions.query = (parameters) => {
		if (parameters.name === 'notifications') {
			return Promise.resolve({ state: Notification.permission });
		}
		return originalQuery(parameters);
	};
	`

	_, err := page.EvalOnNewDocument(js)
	return err
}

// configureBrowser clears the default device emulation so the browser uses
// its own viewport settings instead of emulating a specific device.
func configureBrowser(browser *rod.Browser) {
	browser.NoDefaultDevice()
}

// setViewport sets a realistic viewport on the page.
func setViewport(page *rod.Page) error {
	return page.SetViewport(&proto.EmulationSetDeviceMetricsOverride{
		Width:             1920,
		Height:            1080,
		DeviceScaleFactor: 1,
		Mobile:            false,
	})
}
