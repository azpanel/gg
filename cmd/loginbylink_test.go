package cmd

import (
	"encoding/base64"
	"testing"

	"github.com/mzz2017/gg/dialer"
	_ "github.com/mzz2017/gg/dialer/shadowsocks"
	"github.com/sirupsen/logrus"
)

func TestValidateHTTPSURL(t *testing.T) {
	for _, valid := range []string{
		"https://example.com/sub",
		"https://example.com/sub?token=secret",
	} {
		if err := validateHTTPSURL(valid); err != nil {
			t.Fatalf("validateHTTPSURL(%q): %v", valid, err)
		}
	}
	for _, invalid := range []string{
		"http://example.com/sub",
		"example.com/sub",
		"https://",
	} {
		if err := validateHTTPSURL(invalid); err == nil {
			t.Fatalf("validateHTTPSURL(%q) unexpectedly succeeded", invalid)
		}
	}
}

func TestResolveV2rayNLinks(t *testing.T) {
	links := "ss://YWVzLTEyOC1nY206MQ@example.com:17247#Alpha\ninvalid://node\n"
	for name, encoded := range map[string]string{
		"standard": base64.StdEncoding.EncodeToString([]byte(links)),
		"url-safe": base64.RawURLEncoding.EncodeToString([]byte(links)),
	} {
		t.Run(name, func(t *testing.T) {
			nodes, err := resolveV2rayNLinks(logrus.New(), &dialer.GlobalOption{}, []byte(encoded))
			if err != nil {
				t.Fatal(err)
			}
			if len(nodes) != 1 || nodes[0].Name() != "Alpha" {
				t.Fatalf("unexpected nodes: %#v", nodes)
			}
		})
	}
}

func TestFilterIPv6NodesIsCaseInsensitive(t *testing.T) {
	nodes := []*dialer.Dialer{
		dialer.NewDialer(nil, false, "Alpha", "test", "alpha"),
		dialer.NewDialer(nil, false, "IPv6 Tokyo", "test", "v6-1"),
		dialer.NewDialer(nil, false, "backup-ipV6", "test", "v6-2"),
	}
	kept, filtered := filterIPv6Nodes(nodes)
	if filtered != 2 || len(kept) != 1 || kept[0].Name() != "Alpha" {
		t.Fatalf("kept=%v filtered=%d", kept, filtered)
	}
}
