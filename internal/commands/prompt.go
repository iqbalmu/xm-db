package commands

import (
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/AlecAivazis/survey/v2"
	"github.com/AlecAivazis/survey/v2/terminal"
	"github.com/fatih/color"
	"golang.org/x/term"
)

// Additional keys for text prompts beyond the selector keys in select.go.
const (
	keyBackspace key = 10 + iota
	keyRune
)

// askText shows a text prompt. Enter accepts (the default when empty), Esc
// cancels (ok=false) and Ctrl+C leaves the wizard, exactly like the menus.
// Invalid input is rejected inline; the typed value stays editable.
// hidden renders typed characters as asterisks (passwords).
func askText(message, def string, hidden bool, validate func(string) error) (string, bool, error) {
	fd := int(os.Stdin.Fd())
	if !term.IsTerminal(fd) {
		return askTextSurvey(message, def, validate)
	}

	state, err := term.MakeRaw(fd)
	if err != nil {
		return askTextSurvey(message, def, validate)
	}
	defer term.Restore(fd, state)
	setReadTimeout(fd)

	value := ""
	for {
		drawInput(message, value, hidden)

		k, b, err := promptKey()
		if err != nil {
			clearLine()
			return "", false, err
		}

		switch k {
		case keyEnter:
			pick := value
			if pick == "" {
				pick = def
			}
			if validate != nil {
				if verr := validate(pick); verr != nil {
					clearLine()
					color.Red("  %v", verr)
					value = pick
					continue
				}
			}
			clearLine()
			printAnswer(message, maskInput(pick, hidden))
			return pick, true, nil
		case keyEsc:
			clearLine()
			fmt.Println()
			return "", false, nil
		case keyInterrupt:
			clearLine()
			fmt.Println()
			return "", false, errInterrupt
		case keyBackspace:
			if len(value) > 0 {
				value = value[:len(value)-1]
			}
		case keyRune:
			value += string(rune(b))
		}
	}
}

// askPassword shows a text prompt with hidden input.
func askPassword(message string, validate func(string) error) (string, bool, error) {
	return askText(message, "", true, validate)
}

// askConfirm shows a yes/no prompt. Enter accepts the default; Esc cancels.
func askConfirm(message string, def bool) (bool, bool, error) {
	fd := int(os.Stdin.Fd())
	if !term.IsTerminal(fd) {
		var answer bool
		prompt := &survey.Confirm{Message: message, Default: def}
		if err := survey.AskOne(prompt, &answer); err != nil {
			if errors.Is(err, terminal.InterruptErr) {
				fmt.Println()
				os.Exit(130)
			}
			return false, false, err
		}
		return answer, true, nil
	}

	state, err := term.MakeRaw(fd)
	if err != nil {
		var answer bool
		prompt := &survey.Confirm{Message: message, Default: def}
		if err := survey.AskOne(prompt, &answer); err != nil {
			if errors.Is(err, terminal.InterruptErr) {
				fmt.Println()
				os.Exit(130)
			}
			return false, false, err
		}
		return answer, true, nil
	}
	defer term.Restore(fd, state)
	setReadTimeout(fd)

	hint := "y/N"
	if def {
		hint = "Y/n"
	}

	for {
		fmt.Printf("\r\x1b[2K? %s [%s] ", message, hint)

		k, b, err := promptKey()
		if err != nil {
			clearLine()
			return false, false, err
		}

		accept := func(choice bool) (bool, bool, error) {
			clearLine()
			printAnswer(message, map[bool]string{true: "Yes", false: "No"}[choice])
			return choice, true, nil
		}

		switch k {
		case keyEnter:
			return accept(def)
		case keyEsc:
			clearLine()
			fmt.Println()
			return false, false, nil
		case keyInterrupt:
			clearLine()
			fmt.Println()
			return false, false, errInterrupt
		case keyRune:
			if b == 'y' || b == 'Y' {
				return accept(true)
			}
			if b == 'n' || b == 'N' {
				return accept(false)
			}
		}
	}
}

// promptKey reads the next meaningful keypress for a text prompt: a
// printable byte (keyRune + b), keyEnter, keyBackspace, keyEsc or
// keyInterrupt. Escape sequences (arrows, F-keys) are swallowed.
func promptKey() (key, byte, error) {
	for {
		c, err := blockingReadByte()
		if err != nil {
			return keyIgnore, 0, err
		}
		switch c {
		case 0x03:
			return keyInterrupt, 0, nil
		case 0x0a, 0x0d:
			return keyEnter, 0, nil
		case 0x7f, 0x08:
			return keyBackspace, 0, nil
		case 0x1b:
			// A lone Esc is told apart from escape sequences by the tty
			// inter-byte timeout, exactly like readKey in the selector.
			n, err := readByteTimeout()
			if err != nil {
				return keyIgnore, 0, err
			}
			if !n.ok {
				return keyEsc, 0, nil
			}
			if n.b == '[' || n.b == 'O' {
				d, err := readByteTimeout()
				if err != nil {
					return keyIgnore, 0, err
				}
				_ = d
			}
			drainInput()
			continue
		}
		if c >= 0x20 && c < 0x7f {
			return keyRune, c, nil
		}
	}
}

// drawInput renders the prompt line in place.
func drawInput(message, value string, hidden bool) {
	fmt.Printf("\r\x1b[2K? %s: %s", message, maskInput(value, hidden))
}

func maskInput(value string, hidden bool) string {
	if hidden && value != "" {
		return strings.Repeat("*", len(value))
	}
	return value
}

func clearLine() {
	fmt.Print("\r\x1b[2K")
}

// askTextSurvey is the survey fallback for non-tty stdin, where raw mode is
// unavailable.
func askTextSurvey(message, def string, validate func(string) error) (string, bool, error) {
	var value string
	prompt := &survey.Input{Message: message, Default: def}
	opts := []survey.AskOpt{}
	if validate != nil {
		opts = append(opts, survey.WithValidator(func(ans interface{}) error {
			return validate(ans.(string))
		}))
	}
	if err := survey.AskOne(prompt, &value, opts...); err != nil {
		if errors.Is(err, terminal.InterruptErr) {
			fmt.Println()
			os.Exit(130)
		}
		return "", false, err
	}
	return value, true, nil
}
