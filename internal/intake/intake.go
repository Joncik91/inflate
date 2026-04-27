// Package intake runs the one-time first-run wizard.
package intake

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"strings"

	"golang.org/x/term"

	"github.com/Joncik91/inflate/internal/config"
)

// KeyReader reads a hidden API key from the user. Real callers use a
// terminal-backed implementation; tests inject a stub.
type KeyReader interface {
	ReadKey(prompt string) (string, error)
}

// SetupResult bundles everything the first-run wizard collects.
type SetupResult struct {
	Profile     config.Profile
	Provider    config.ProviderConfig
	APIKeyName  string // env var name (e.g. "DEEPSEEK_API_KEY")
	APIKeyValue string // the secret itself
}

// RunFromReader keeps the 3-question profile-only flow for callers that
// don't need provider setup. Existing tests rely on this signature.
func RunFromReader(r io.Reader, w io.Writer) (config.Profile, error) {
	scanner := bufio.NewScanner(r)
	return runProfile(scanner, w), nil
}

// RunFullSetup asks the 3 profile questions plus provider + API key questions.
// keys reads the API key with hidden input. Returns everything needed to
// persist profile.toml, config.toml, and .env.
// A single bufio.Scanner is shared for all reads so buffered input is not lost
// between the profile and provider phases.
func RunFullSetup(r io.Reader, w io.Writer, keys KeyReader) (SetupResult, error) {
	scanner := bufio.NewScanner(r)
	prof := runProfile(scanner, w)

	provChoice := strings.ToLower(askWithScanner(scanner, w,
		"Which LLM provider? [a]nthropic / [d]eepseek / [o]penai / [g]oogle / [c]ustom (openai-compat URL)"))

	prov := config.ProviderConfig{}
	keyName := ""
	switch provChoice {
	case "a", "anthropic":
		prov.Kind = "anthropic"
		prov.Model = askWithScanner(scanner, w, "Model? (e.g. claude-haiku-4-5, claude-sonnet-4-6, claude-opus-4-7)")
		keyName = "ANTHROPIC_API_KEY"
	case "d", "deepseek":
		prov.Kind = "openai_compat"
		prov.BaseURL = "https://api.deepseek.com/v1"
		prov.Model = askWithScanner(scanner, w, "Model? (e.g. deepseek-chat)")
		keyName = "DEEPSEEK_API_KEY"
	case "o", "openai":
		prov.Kind = "openai_compat"
		prov.BaseURL = "https://api.openai.com/v1"
		prov.Model = askWithScanner(scanner, w, "Model? (e.g. gpt-5-mini)")
		keyName = "OPENAI_API_KEY"
	case "g", "google":
		prov.Kind = "google"
		prov.Model = askWithScanner(scanner, w, "Model? (e.g. gemini-2.0-flash)")
		keyName = "GOOGLE_API_KEY"
	case "c", "custom":
		prov.Kind = "openai_compat"
		prov.BaseURL = askWithScanner(scanner, w, "Base URL? (e.g. https://my.host/v1)")
		prov.Model = askWithScanner(scanner, w, "Model name?")
		keyName = askWithScanner(scanner, w, "Env var name for the key? (e.g. MY_LOCAL_KEY)")
	default:
		return SetupResult{}, fmt.Errorf("unknown provider choice %q", provChoice)
	}
	prov.APIKeyEnv = keyName

	keyVal, err := keys.ReadKey(fmt.Sprintf("Paste your %s (input hidden)", keyName))
	if err != nil {
		return SetupResult{}, err
	}
	keyVal = strings.TrimSpace(keyVal)

	return SetupResult{
		Profile:     prof,
		Provider:    prov,
		APIKeyName:  keyName,
		APIKeyValue: keyVal,
	}, nil
}

// TerminalKeyReader reads from the terminal with echo disabled. Real-run path.
type TerminalKeyReader struct{}

func (TerminalKeyReader) ReadKey(prompt string) (string, error) {
	fmt.Fprint(os.Stdout, prompt+": ")
	b, err := term.ReadPassword(int(os.Stdin.Fd()))
	fmt.Fprintln(os.Stdout)
	if err != nil {
		return "", err
	}
	return string(b), nil
}

// runProfile reads 3 profile questions using an existing scanner.
func runProfile(scanner *bufio.Scanner, w io.Writer) config.Profile {
	identity := askWithScanner(scanner, w, "Who are you? (e.g., senior backend engineer, mostly Go and Python)")
	work := askWithScanner(scanner, w, "What kind of work? (e.g., API services, CLI tools)")
	style := normalizeStyle(askWithScanner(scanner, w, "Prompt style preference? terse / standard / verbose"))
	if identity == "" {
		identity = "developer"
	}
	if work == "" {
		work = "general software engineering"
	}
	return config.Profile{Identity: identity, Work: work, Style: style}
}

// askWithScanner prints q to w, reads the next line from scanner, returns it trimmed.
func askWithScanner(scanner *bufio.Scanner, w io.Writer, q string) string {
	fmt.Fprintln(w, q)
	if scanner.Scan() {
		return strings.TrimSpace(scanner.Text())
	}
	return ""
}

func normalizeStyle(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	switch s {
	case "terse", "acolyte":
		return "terse"
	case "verbose", "grandmaster":
		return "verbose"
	default:
		return "standard"
	}
}
