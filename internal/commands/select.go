package commands

import (
	"errors"
	"fmt"
	"os"

	"github.com/AlecAivazis/survey/v2"
	"github.com/AlecAivazis/survey/v2/terminal"
	"github.com/fatih/color"
	"github.com/mattn/go-runewidth"
	"golang.org/x/term"
)

// errInterrupt reports that the user pressed Ctrl+C and wants to leave the
// wizard, as opposed to Esc which only backs out of the current menu.
var errInterrupt = errors.New("interrupted")

const fallbackWidth = 80

// promptSelect shows an interactive single-choice selector and returns the
// chosen option. It returns false when the user pressed Esc (back/cancel);
// Ctrl+C leaves the wizard.
func promptSelect(message string, options []string) (string, bool) {
	return promptSelectDefault(message, options, "")
}

// promptSelectDefault is promptSelect with one option pre-highlighted
// (matched by text); an empty or missing def highlights the first option.
func promptSelectDefault(message string, options []string, def string) (string, bool) {
	initial := 0
	for i, o := range options {
		if o == def {
			initial = i
			break
		}
	}

	idx, ok, err := selectOption(message, options, initial)
	if err != nil {
		if errors.Is(err, errInterrupt) {
			fmt.Println()
			os.Exit(130)
		}
		color.Red("Error: %v", err)
		return "", false
	}
	if !ok {
		return "", false
	}
	return options[idx], true
}

// selectOption renders the selector and returns the chosen index. Esc yields
// ok=false, Ctrl+C yields errInterrupt. initial is the pre-highlighted index.
func selectOption(message string, options []string, initial int) (int, bool, error) {
	fd := int(os.Stdin.Fd())
	if !term.IsTerminal(fd) {
		return fallbackSelect(message, options, initial)
	}

	state, err := term.MakeRaw(fd)
	if err != nil {
		return fallbackSelect(message, options, initial)
	}
	defer term.Restore(fd, state)
	setReadTimeout(fd)

	width := termWidth(fd)
	sel := initial
	if sel < 0 || sel >= len(options) {
		sel = 0
	}
	renderSelect(message, options, sel, width, false)

	for {
		k, err := readKey()
		if err != nil {
			eraseSelect(message, options)
			return 0, false, err
		}

		switch k {
		case keyUp:
			if sel > 0 {
				sel--
			} else {
				continue
			}
		case keyDown:
			if sel < len(options)-1 {
				sel++
			} else {
				continue
			}
		case keyEnter:
			eraseSelect(message, options)
			printAnswer(message, options[sel])
			return sel, true, nil
		case keyEsc:
			eraseSelect(message, options)
			return 0, false, nil
		case keyInterrupt:
			eraseSelect(message, options)
			return 0, false, errInterrupt
		default: // keyIgnore
			continue
		}

		renderSelect(message, options, sel, width, true)
	}
}

// key is a logical keypress.
type key int

const (
	keyIgnore key = iota
	keyUp
	keyDown
	keyEnter
	keyEsc
	keyInterrupt
)

// blockingReadByte waits until input arrives and returns one byte.
func blockingReadByte() (byte, error) {
	for {
		c, err := readByteTimeout()
		if err != nil {
			return 0, err
		}
		if c.ok {
			return c.b, nil
		}
	}
}

// readKey maps raw bytes to logical keys. A lone ESC is told apart from the
// escape sequences terminals send for arrow keys by the tty inter-byte
// timeout configured in setReadTimeout.
func readKey() (key, error) {
	b, err := blockingReadByte()
	if err != nil {
		return 0, err
	}

	switch b {
	case 0x03: // Ctrl+C
		return keyInterrupt, nil
	case 0x0a, 0x0d: // Enter
		return keyEnter, nil
	case 0x1b: // ESC or start of an escape sequence
		c, err := readByteTimeout()
		if err != nil {
			return 0, err
		}
		if !c.ok {
			return keyEsc, nil
		}
		if c.b == '[' || c.b == 'O' {
			d, err := readByteTimeout()
			if err != nil {
				return 0, err
			}
			if d.ok {
				switch d.b {
				case 'A':
					return keyUp, nil
				case 'B':
					return keyDown, nil
				}
			}
		}
		// Unhandled sequence (F1-F12, Alt+key, ...): swallow the rest.
		drainInput()
		return keyIgnore, nil
	}
	return keyIgnore, nil
}

// drainInput discards buffered input until the tty goes quiet.
func drainInput() {
	for {
		c, err := readByteTimeout()
		if err != nil || !c.ok {
			return
		}
	}
}

// fallbackSelect keeps the survey behavior for non-tty stdin, where raw
// mode is not available.
func fallbackSelect(message string, options []string, initial int) (int, bool, error) {
	var choice string
	prompt := &survey.Select{
		Message: message,
		Options: options,
		Default: options[initial],
	}
	if err := survey.AskOne(prompt, &choice); err != nil {
		if err == terminal.InterruptErr {
			return 0, false, errInterrupt
		}
		return 0, false, err
	}
	for i, o := range options {
		if o == choice {
			return i, true, nil
		}
	}
	return 0, false, fmt.Errorf("unexpected choice %q", choice)
}

// renderSelect draws the prompt and options; sel is highlighted.
func renderSelect(message string, options []string, sel int, width int, redraw bool) {
	out := os.Stdout
	if redraw {
		fmt.Fprintf(out, "\x1b[%dA\r", len(options)+1)
	}
	fmt.Fprintf(out, "\x1b[2K? %s\r\n", message)
	for i, o := range options {
		line := "  " + o
		if i == sel {
			line = "> " + o
		}
		fmt.Fprintf(out, "\x1b[2K%s\r\n", runewidth.Truncate(line, width, "…"))
	}
}

// eraseSelect removes the rendered selector, leaving the cursor where the
// prompt line was so the parent menu redraws in place.
func eraseSelect(message string, options []string) {
	fmt.Fprintf(os.Stdout, "\x1b[%dA\r\x1b[J", len(options)+1)
}

func printAnswer(message string, answer string) {
	fmt.Printf("%s %s %s\r\n",
		color.New(color.FgGreen, color.Bold).Sprint("?"),
		color.New(color.Bold).Sprint(message),
		color.New(color.FgCyan).Sprint(answer),
	)
}

// termWidth returns the terminal width, falling back to 80 columns.
func termWidth(fd int) int {
	w, _, err := term.GetSize(fd)
	if err != nil || w <= 0 {
		return fallbackWidth
	}
	return w
}
