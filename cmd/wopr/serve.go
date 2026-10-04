package main

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"os"
	"strconv"
	"strings"

	"github.com/alexrudloff/wopr/internal/codingagent"
	"github.com/alexrudloff/wopr/internal/codingagent/serve"
)

func runServeCommand(args []string) int {
	if len(args) == 0 || args[0] != "serve" {
		return -1
	}
	args = args[1:]
	if len(args) > 0 {
		switch args[0] {
		case "-h", "--help":
			fmt.Println("Usage:\n  wopr serve [--port N] [--hostname H]\n\nStart the wopr serve daemon.")
			return 0
		}
	}

	flagPort := 0
	flagHost := ""

	var portVal, hostVal string
	stringFlags := map[string]*string{
		"--port":     &portVal,
		"--hostname": &hostVal,
	}

	for i := 0; i < len(args); i++ {
		arg := args[i]
		if p, ok := stringFlags[arg]; ok {
			if i+1 >= len(args) || args[i+1] == "" {
				fmt.Fprintf(os.Stderr, "wopr serve: %s requires a non-empty value\n", arg)
				fmt.Fprintln(os.Stderr, "Usage: wopr serve [--port N] [--hostname H]")
				return 2
			}
			i++
			*p = args[i]
			continue
		}
		switch {
		case strings.HasPrefix(arg, "-"):
			fmt.Fprintf(os.Stderr, "wopr serve: unknown option: %s\n", arg)
			fmt.Fprintln(os.Stderr, "Usage: wopr serve [--port N] [--hostname H]")
			return 2
		default:
			fmt.Fprintf(os.Stderr, "wopr serve: unexpected argument: %s\n", arg)
			fmt.Fprintln(os.Stderr, "Usage: wopr serve [--port N] [--hostname H]")
			return 2
		}
	}

	if portVal != "" {
		p, err := strconv.Atoi(portVal)
		if err != nil || p <= 0 {
			fmt.Fprintf(os.Stderr, "wopr serve: --port requires a positive integer\n")
			fmt.Fprintln(os.Stderr, "Usage: wopr serve [--port N] [--hostname H]")
			return 2
		}
		flagPort = p
	}
	flagHost = hostVal

	cwd, err := os.Getwd()
	if err != nil {
		return failf("wopr serve: %v", err)
	}
	agentDir := codingagent.AgentDir()
	globalSettings := codingagent.NewSettingsManagerWithProjectTrust(cwd, agentDir, false)
	projectTrusted, err := resolveProjectTrusted(context.Background(), projectTrustResolutionOptions{
		CWD: cwd, Store: codingagent.NewProjectTrustStore(agentDir),
		Default: globalSettings.GetDefaultProjectTrust(),
	})
	if err != nil {
		return failf("wopr serve: %v", err)
	}
	settings := codingagent.NewSettingsManagerWithProjectTrust(cwd, agentDir, projectTrusted)
	reportSettingsErrors(settings, "serve")

	port := settings.Get().GetServerPort()
	if flagPort != 0 {
		port = flagPort
	}
	host := settings.Get().GetServerHostname()
	if flagHost != "" {
		host = flagHost
	}

	if nonLoopbackBind(host) {
		fmt.Fprintf(os.Stderr, "wopr serve: no authentication. Anyone who can reach %s can run tools and read files as this user. Binding to a LAN is not a security control.\n", net.JoinHostPort(host, strconv.Itoa(port)))
	}

	srv, ln, err := serve.New(net.JoinHostPort(host, strconv.Itoa(port)))
	if err != nil {
		return failf("wopr serve: %v", err)
	}

	fmt.Fprintf(os.Stderr, "wopr serve listening on http://%s\n", ln.Addr().String())
	if err := srv.Serve(ln); err != nil && err != http.ErrServerClosed {
		return failf("wopr serve: %v", err)
	}
	return 0
}

func nonLoopbackBind(host string) bool {
	if host == "localhost" {
		return false
	}
	ip := net.ParseIP(host)
	return ip == nil || !ip.IsLoopback()
}
