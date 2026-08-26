package cmd

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/mzz2017/gg/config"
	"github.com/mzz2017/gg/dialer"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"golang.org/x/term"
)

const myIPURL = "http://myip.ipip.net"

var errSelectionCanceled = errors.New("node selection canceled")

var selectCmd = &cobra.Command{
	Use:   "select",
	Short: "Select a node from the saved subscription",
	Args:  cobra.NoArgs,
	RunE:  runSelect,
}

func runSelect(cmd *cobra.Command, _ []string) error {
	log := NewLogger(verbose)
	v, configPath := getConfig(log, true, viper.New, nil)
	if len(config.ParamsObj.Subscription.Nodes) == 0 {
		return fmt.Errorf("no saved subscription nodes; run 'gg loginbylink' first")
	}

	opt := &dialer.GlobalOption{AllowInsecure: config.ParamsObj.AllowInsecure}
	nodes := make([]*dialer.Dialer, 0, len(config.ParamsObj.Subscription.Nodes))
	for _, link := range config.ParamsObj.Subscription.Nodes {
		node, err := GetDialerFromLink(link, opt, false, "")
		if err != nil {
			log.Tracef("skip invalid saved node: %v\n", err)
			continue
		}
		nodes = append(nodes, node)
	}
	if len(nodes) == 0 {
		return fmt.Errorf("the saved subscription contains no supported nodes; run 'gg loginbylink' again")
	}
	sortNodesByName(nodes)

	for {
		selected, err := selectSavedNode(nodes)
		if err != nil {
			return err
		}
		body, err := fetchThroughNode(cmd.Context(), selected, myIPURL)
		if err != nil {
			fmt.Printf("Node %q is unavailable: %v\n", selected.Name(), err)
			fmt.Print("Press Enter to select another node...")
			if waitErr := waitForEnter(os.Stdin); waitErr != nil {
				return waitErr
			}
			continue
		}
		fmt.Print(string(body))
		if len(body) > 0 && body[len(body)-1] != '\n' {
			fmt.Println()
		}
		fmt.Printf("Node %q is available.\n", selected.Name())

		settings := v.AllSettings()
		if err := config.SetValueHierarchicalMap(settings, "node", selected.Link()); err != nil {
			return err
		}
		if err := WriteConfig(settings, configPath); err != nil {
			return fmt.Errorf("save selected node: %w", err)
		}
		return nil
	}
}

func sortNodesByName(nodes []*dialer.Dialer) {
	sort.SliceStable(nodes, func(i, j int) bool {
		left := strings.ToLower(nodes[i].Name())
		right := strings.ToLower(nodes[j].Name())
		if left == right {
			return nodes[i].Link() < nodes[j].Link()
		}
		return left < right
	})
}

func fetchThroughNode(parent context.Context, node *dialer.Dialer, target string) ([]byte, error) {
	ctx, cancel := context.WithTimeout(parent, 15*time.Second)
	defer cancel()
	contextDialer := &dialer.ContextDialer{Dialer: node.Dialer}
	client := &http.Client{
		Transport: &http.Transport{DialContext: contextDialer.DialContext},
		Timeout:   15 * time.Second,
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, target, nil)
	if err != nil {
		return nil, err
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 400 {
		return nil, fmt.Errorf("unexpected HTTP status: %s", resp.Status)
	}
	return io.ReadAll(resp.Body)
}

type selectionKey int

const (
	keyUnknown selectionKey = iota
	keyUp
	keyDown
	keyPageUp
	keyPageDown
	keyEnter
	keyCancel
)

func selectSavedNode(nodes []*dialer.Dialer) (*dialer.Dialer, error) {
	stdinFD := int(os.Stdin.Fd())
	if !term.IsTerminal(stdinFD) {
		return nil, fmt.Errorf("node selection requires an interactive terminal")
	}
	oldState, err := term.MakeRaw(stdinFD)
	if err != nil {
		return nil, err
	}
	defer term.Restore(stdinFD, oldState)

	pageSize := 10
	if _, height, sizeErr := term.GetSize(int(os.Stdout.Fd())); sizeErr == nil && height > 8 {
		pageSize = height - 6
	}
	if pageSize > len(nodes) {
		pageSize = len(nodes)
	}
	cursor := 0
	reader := bufio.NewReader(os.Stdin)
	for {
		renderNodePage(os.Stdout, nodes, cursor, pageSize)
		switch readSelectionKey(reader) {
		case keyUp:
			if cursor > 0 {
				cursor--
			}
		case keyDown:
			if cursor < len(nodes)-1 {
				cursor++
			}
		case keyPageUp:
			cursor -= pageSize
			if cursor < 0 {
				cursor = 0
			}
		case keyPageDown:
			cursor += pageSize
			if cursor >= len(nodes) {
				cursor = len(nodes) - 1
			}
		case keyEnter:
			fmt.Fprint(os.Stdout, "\x1b[2J\x1b[H")
			return nodes[cursor], nil
		case keyCancel:
			return nil, errSelectionCanceled
		}
	}
}

func renderNodePage(w io.Writer, nodes []*dialer.Dialer, cursor, pageSize int) {
	start := (cursor / pageSize) * pageSize
	end := start + pageSize
	if end > len(nodes) {
		end = len(nodes)
	}
	fmt.Fprint(w, "\x1b[2J\x1b[H")
	fmt.Fprintf(w, "Select Node (%d/%d)\r\n", cursor+1, len(nodes))
	fmt.Fprintln(w, "Use ↑/↓ to move, PgUp/PgDn to change page, Enter to select, Ctrl+C to cancel.\r")
	for i := start; i < end; i++ {
		prefix := "  "
		if i == cursor {
			prefix = "> "
		}
		fmt.Fprintf(w, "%s%s\r\n", prefix, nodes[i].Name())
	}
}

func readSelectionKey(r *bufio.Reader) selectionKey {
	b, err := r.ReadByte()
	if err != nil {
		return keyCancel
	}
	switch b {
	case 3:
		return keyCancel
	case '\r', '\n':
		return keyEnter
	case 0, 224: // Windows console extended key prefix.
		next, _ := r.ReadByte()
		switch next {
		case 72:
			return keyUp
		case 80:
			return keyDown
		case 73:
			return keyPageUp
		case 81:
			return keyPageDown
		}
	case 27:
		second, _ := r.ReadByte()
		if second != '[' {
			return keyCancel
		}
		third, _ := r.ReadByte()
		switch third {
		case 'A':
			return keyUp
		case 'B':
			return keyDown
		case '5', '6':
			fourth, _ := r.ReadByte()
			if fourth == '~' {
				if third == '5' {
					return keyPageUp
				}
				return keyPageDown
			}
		}
	}
	return keyUnknown
}

func waitForEnter(r io.Reader) error {
	reader := bufio.NewReader(r)
	for {
		b, err := reader.ReadByte()
		if err != nil {
			return err
		}
		if b == '\r' || b == '\n' {
			return nil
		}
		if b == 3 {
			return errSelectionCanceled
		}
	}
}
