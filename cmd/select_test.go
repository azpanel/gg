package cmd

import (
	"bufio"
	"context"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/mzz2017/gg/dialer"
)

func TestReadSelectionKey(t *testing.T) {
	tests := map[string]struct {
		input string
		want  selectionKey
	}{
		"up":        {"\x1b[A", keyUp},
		"down":      {"\x1b[B", keyDown},
		"page up":   {"\x1b[5~", keyPageUp},
		"page down": {"\x1b[6~", keyPageDown},
		"enter":     {"\r", keyEnter},
		"cancel":    {"\x03", keyCancel},
	}
	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			got := readSelectionKey(bufio.NewReader(strings.NewReader(test.input)))
			if got != test.want {
				t.Fatalf("got %v, want %v", got, test.want)
			}
		})
	}
}

func TestFetchThroughNode(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("IP response"))
	}))
	defer server.Close()

	node := dialer.NewDialer(&net.Dialer{}, false, "direct-test", "test", "test://node")
	body, err := fetchThroughNode(context.Background(), node, server.URL)
	if err != nil {
		t.Fatal(err)
	}
	if string(body) != "IP response" {
		t.Fatalf("unexpected response: %q", body)
	}
}

func TestSortNodesByName(t *testing.T) {
	nodes := []*dialer.Dialer{
		dialer.NewDialer(nil, false, "zulu", "test", "z"),
		dialer.NewDialer(nil, false, "Alpha", "test", "a"),
		dialer.NewDialer(nil, false, "beta", "test", "b"),
	}
	sortNodesByName(nodes)
	got := []string{nodes[0].Name(), nodes[1].Name(), nodes[2].Name()}
	if strings.Join(got, ",") != "Alpha,beta,zulu" {
		t.Fatalf("unexpected order: %v", got)
	}
}

func TestWaitForEnter(t *testing.T) {
	if err := waitForEnter(strings.NewReader("ignored\n")); err != nil {
		t.Fatal(err)
	}
	if err := waitForEnter(strings.NewReader("\x03")); err != errSelectionCanceled {
		t.Fatalf("got %v, want %v", err, errSelectionCanceled)
	}
}
