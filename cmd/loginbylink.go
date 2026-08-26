package cmd

import (
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/AlecAivazis/survey/v2"
	"github.com/mzz2017/gg/common"
	"github.com/mzz2017/gg/config"
	"github.com/mzz2017/gg/dialer"
	"github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

const ipv6ProbeURL = "https://ipv6.ip.sb"

var loginByLinkCmd = &cobra.Command{
	Use:   "loginbylink",
	Short: "Login with a v2rayN subscription link",
	Args:  cobra.NoArgs,
	RunE:  runLoginByLink,
}

func runLoginByLink(cmd *cobra.Command, _ []string) error {
	var subscription string
	if err := survey.AskOne(&survey.Input{
		Message: "Enter the HTTPS subscription link:",
	}, &subscription, common.SetRequire); err != nil {
		return err
	}
	subscription = strings.TrimSpace(subscription)
	if err := validateHTTPSURL(subscription); err != nil {
		return err
	}

	log := NewLogger(verbose)
	v, configPath := getConfig(log, true, viper.New, nil)
	body, err := fetchSubscription(cmd.Context(), subscription)
	if err != nil {
		return fmt.Errorf("pull subscription: %w", err)
	}
	nodes, err := resolveV2rayNLinks(log, &dialer.GlobalOption{AllowInsecure: config.ParamsObj.AllowInsecure}, body)
	if err != nil {
		return fmt.Errorf("parse subscription: %w", err)
	}

	filtered := 0
	if !hasIPv6Connectivity(cmd.Context()) {
		nodes, filtered = filterIPv6Nodes(nodes)
	}
	if len(nodes) == 0 {
		return fmt.Errorf("no usable nodes found in the subscription")
	}

	links := make([]string, 0, len(nodes))
	for _, node := range nodes {
		links = append(links, node.Link())
	}
	settings := v.AllSettings()
	if err := config.SetValueHierarchicalMap(settings, "subscription.link", subscription); err != nil {
		return err
	}
	if err := config.SetValueHierarchicalMap(settings, "subscription.nodes", links); err != nil {
		return err
	}
	if err := config.SetValueHierarchicalMap(settings, "node", ""); err != nil {
		return err
	}
	if err := config.SetValueHierarchicalMap(settings, "cache.subscription.last_node", ""); err != nil {
		return err
	}
	if err := WriteConfig(settings, configPath); err != nil {
		return fmt.Errorf("save subscription: %w", err)
	}

	if filtered > 0 {
		fmt.Printf("Filtered %d IPv6 nodes because IPv6 is unavailable.\n", filtered)
	}
	fmt.Printf("Parsed %d nodes.\n", len(nodes))
	return nil
}

func filterIPv6Nodes(nodes []*dialer.Dialer) ([]*dialer.Dialer, int) {
	kept := nodes[:0]
	filtered := 0
	for _, node := range nodes {
		if strings.Contains(strings.ToLower(node.Name()), "ipv6") {
			filtered++
			continue
		}
		kept = append(kept, node)
	}
	return kept, filtered
}

func validateHTTPSURL(raw string) error {
	u, err := url.ParseRequestURI(raw)
	if err != nil || u.Scheme != "https" || u.Host == "" {
		return fmt.Errorf("subscription link must be a valid HTTPS URL")
	}
	return nil
}

func fetchSubscription(ctx context.Context, subscription string) ([]byte, error) {
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, subscription, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "v2rayN")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("unexpected HTTP status: %s", resp.Status)
	}
	return io.ReadAll(resp.Body)
}

func resolveV2rayNLinks(log *logrus.Logger, opt *dialer.GlobalOption, encoded []byte) ([]*dialer.Dialer, error) {
	raw, err := common.Base64StdDecode(string(encoded))
	if err != nil {
		raw, err = common.Base64URLDecode(string(encoded))
		if err != nil {
			return nil, fmt.Errorf("invalid base64 data")
		}
	}

	var nodes []*dialer.Dialer
	for _, line := range strings.Split(raw, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		node, parseErr := GetDialerFromLink(line, opt, false, "")
		if parseErr != nil {
			log.Tracef("skip invalid node: %v: %v\n", parseErr, line)
			continue
		}
		nodes = append(nodes, node)
	}
	if len(nodes) == 0 {
		return nil, fmt.Errorf("subscription contains no supported node links")
	}
	return nodes, nil
}

func hasIPv6Connectivity(parent context.Context) bool {
	ctx, cancel := context.WithTimeout(parent, 10*time.Second)
	defer cancel()
	d := &net.Dialer{Timeout: 10 * time.Second}
	client := &http.Client{
		Transport: &http.Transport{
			DialContext: func(ctx context.Context, _, address string) (net.Conn, error) {
				return d.DialContext(ctx, "tcp6", address)
			},
		},
		Timeout: 10 * time.Second,
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, ipv6ProbeURL, nil)
	if err != nil {
		return false
	}
	resp, err := client.Do(req)
	if err != nil {
		return false
	}
	defer resp.Body.Close()
	return resp.StatusCode >= 200 && resp.StatusCode < 400
}
