// Package cookies provides generic browser cookie extraction via kooky.
// Safari-first, Chrome as fallback.
package cookies

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"runtime"

	"github.com/browserutils/kooky"
	"github.com/browserutils/kooky/browser/chrome"
	"github.com/browserutils/kooky/browser/safari"
)

// Spec describes which cookies to extract for a given domain.
type Spec struct {
	Domain string   // domain suffix to match (e.g. "perplexity.ai")
	Names  []string // cookie names to extract
}

// Result holds extracted cookies.
type Result struct {
	Cookies map[string]string // name -> value
	Browser string            // which browser provided them
}

// HasAll reports whether all requested cookie names were found.
func (r *Result) HasAll(names []string) bool {
	if r == nil {
		return false
	}
	for _, n := range names {
		if r.Cookies[n] == "" {
			return false
		}
	}
	return true
}

// Extract reads cookies matching the spec from browsers.
// Order: Safari first, then Chrome (Safari cookies are plaintext on macOS,
// Chrome requires Keychain access).
func Extract(ctx context.Context, spec Spec, logf func(string, ...any)) (*Result, error) {
	if logf == nil {
		logf = func(string, ...any) {}
	}

	result := &Result{Cookies: make(map[string]string)}
	nameSet := make(map[string]bool, len(spec.Names))
	for _, n := range spec.Names {
		nameSet[n] = true
	}

	// Safari first (no Keychain prompt).
	if err := extractSafari(ctx, spec.Domain, nameSet, result, logf); err != nil {
		logf("  Safari: %v", err)
	}
	if result.HasAll(spec.Names) {
		return result, nil
	}

	// Chrome fallback.
	if err := extractChrome(ctx, spec.Domain, nameSet, result, logf); err != nil {
		logf("  Chrome: %v", err)
	}

	return result, nil
}

// ExtractMulti extracts cookies for multiple specs at once.
// It stops searching additional specs once all unique cookie names are found.
func ExtractMulti(ctx context.Context, specs []Spec, logf func(string, ...any)) (*Result, error) {
	if logf == nil {
		logf = func(string, ...any) {}
	}

	// Collect all unique cookie names across specs.
	allNames := make(map[string]bool)
	for _, spec := range specs {
		for _, n := range spec.Names {
			allNames[n] = true
		}
	}
	result := &Result{Cookies: make(map[string]string)}
	for _, spec := range specs {
		// Skip if we already have every cookie we need.
		haveAll := true
		for n := range allNames {
			if result.Cookies[n] == "" {
				haveAll = false
				break
			}
		}
		if haveAll {
			break
		}
		r, err := Extract(ctx, spec, logf)
		if err != nil {
			logf("  %s: %v", spec.Domain, err)
			continue
		}
		for k, v := range r.Cookies {
			if v != "" {
				result.Cookies[k] = v
				if result.Browser == "" {
					result.Browser = r.Browser
				}
			}
		}
	}
	return result, nil
}

func extractSafari(ctx context.Context, domain string, nameSet map[string]bool, result *Result, logf func(string, ...any)) error {
	paths, err := safariCookiePaths()
	if err != nil {
		return err
	}

	for _, path := range paths {
		if _, err := os.Stat(path); os.IsNotExist(err) {
			continue
		}

		logf("  Searching Safari cookies at %s ...", path)

		seq := safari.TraverseCookies(path,
			kooky.DomainHasSuffix(domain),
		).OnlyCookies()

		for cookie := range seq {
			select {
			case <-ctx.Done():
				return ctx.Err()
			default:
			}
			if cookie == nil || cookie.Value == "" {
				continue
			}
			if len(nameSet) > 0 && !nameSet[cookie.Name] {
				continue
			}
			// Keep the latest-expiring value.
			if existing := result.Cookies[cookie.Name]; existing != "" {
				continue // first match wins for Safari
			}
			result.Cookies[cookie.Name] = cookie.Value
			if result.Browser == "" {
				result.Browser = "safari"
			}
			logf("    Found %s (domain=%s, browser=safari)", cookie.Name, cookie.Domain)
		}
	}

	return nil
}

func extractChrome(ctx context.Context, domain string, nameSet map[string]bool, result *Result, logf func(string, ...any)) error {
	path, err := chromeCookiePath()
	if err != nil {
		return err
	}
	if _, err := os.Stat(path); os.IsNotExist(err) {
		return fmt.Errorf("Chrome cookie file not found at %s", path)
	}

	logf("  Searching Chrome cookies at %s ...", path)

	seq := chrome.TraverseCookies(path,
		kooky.DomainHasSuffix(domain),
	).OnlyCookies()

	for cookie := range seq {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}
		if cookie == nil || cookie.Value == "" {
			continue
		}
		if len(nameSet) > 0 && !nameSet[cookie.Name] {
			continue
		}
		if existing := result.Cookies[cookie.Name]; existing != "" {
			continue
		}
		result.Cookies[cookie.Name] = cookie.Value
		if result.Browser == "" {
			result.Browser = "chrome"
		}
		logf("    Found %s (domain=%s, browser=chrome)", cookie.Name, cookie.Domain)
	}

	return nil
}

func chromeCookiePath() (string, error) {
	if runtime.GOOS != "darwin" {
		return "", fmt.Errorf("unsupported OS %q — only macOS is currently supported", runtime.GOOS)
	}
	dir, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	networkPath := filepath.Join(dir, "Google", "Chrome", "Default", "Network", "Cookies")
	if _, err := os.Stat(networkPath); err == nil {
		return networkPath, nil
	}
	return filepath.Join(dir, "Google", "Chrome", "Default", "Cookies"), nil
}

func safariCookiePaths() ([]string, error) {
	if runtime.GOOS != "darwin" {
		return nil, fmt.Errorf("unsupported OS %q — only macOS is currently supported", runtime.GOOS)
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, err
	}
	return []string{
		filepath.Join(home, "Library", "Containers", "com.apple.Safari", "Data", "Library", "Cookies", "Cookies.binarycookies"),
		filepath.Join(home, "Library", "Cookies", "Cookies.binarycookies"),
	}, nil
}
