package app

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"easy_proxies/internal/boxmgr"
	"easy_proxies/internal/config"
	"easy_proxies/internal/monitor"
	"easy_proxies/internal/subscription"
	"easy_proxies/internal/sysproxy"
)

// Run builds the runtime components from config and blocks until shutdown.
func Run(ctx context.Context, cfg *config.Config, systemProxy bool) error {
	// Build monitor config
	proxyUsername := cfg.Listener.Username
	proxyPassword := cfg.Listener.Password
	if cfg.Mode == "multi-port" || cfg.Mode == "hybrid" {
		proxyUsername = cfg.MultiPort.Username
		proxyPassword = cfg.MultiPort.Password
	}

	monitorCfg := monitor.Config{
		Enabled:          cfg.ManagementEnabled(),
		Listen:           cfg.Management.Listen,
		ProbeTarget:      cfg.Management.ProbeTarget,
		Password:         cfg.Management.Password,
		ProxyUsername:    proxyUsername,
		ProxyPassword:    proxyPassword,
		ExternalIP:       cfg.ExternalIP,
		ProbeConcurrency: cfg.ProbeConcurrencyOrDefault(),
	}

	// Create and start BoxManager
	boxMgr := boxmgr.New(cfg, monitorCfg)
	sysProxy := sysproxy.New()
	target := sysproxy.SystemProxyTarget(cfg.Management.Listen, cfg.Listener.Address, cfg.Listener.Port)
	localProxy := sysproxy.LocalProxyAddress(cfg.Listener.Address, cfg.Listener.Port)
	if !systemProxy {
		// Only remove stale OS proxy state when this process will not take over
		// the PAC. Cleaning before startup can disable a live PAC when another
		// process still owns the listener and this process fails to bind.
		if err := sysProxy.CleanupStale(sysproxy.PACURL(cfg.Management.Listen), localProxy); err != nil {
			log.Printf("⚠️  failed to clean stale system proxy: %v", err)
		}
	}
	if err := boxMgr.Start(ctx); err != nil {
		return fmt.Errorf("start box manager: %w", err)
	}
	defer boxMgr.Close()

	if server := boxMgr.MonitorServer(); server != nil {
		server.SetConfig(cfg)
		server.SetSysProxy(sysProxy)
	}
	if systemProxy {
		if err := sysProxy.Enable(target); err != nil {
			log.Printf("⚠️  failed to enable system proxy: %v", err)
		} else {
			log.Printf("✅ system proxy enabled with %s", target)
			if server := boxMgr.MonitorServer(); server != nil {
				server.SetSysProxyState(true)
			}
		}
	}
	defer func() {
		if err := sysProxy.Disable(); err != nil {
			log.Printf("⚠️  failed to restore system proxy: %v", err)
		}
		if server := boxMgr.MonitorServer(); server != nil {
			server.SetSysProxyState(false)
		}
	}()

	// Always create SubscriptionManager so WebUI can hot-reload subscription config
	subMgr := subscription.New(cfg, boxMgr)
	defer subMgr.Stop()

	// Start refresh loop only if subscriptions are already configured
	if cfg.SubscriptionRefresh.Enabled && len(cfg.Subscriptions) > 0 {
		subMgr.Start()
	}

	// Wire up subscription manager to monitor server for API endpoints
	if server := boxMgr.MonitorServer(); server != nil {
		server.SetSubscriptionRefresher(subMgr)
	}

	// Wait for shutdown signal
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	defer signal.Stop(sigCh)

	select {
	case <-ctx.Done():
		fmt.Println("Context cancelled, initiating graceful shutdown...")
	case sig := <-sigCh:
		fmt.Printf("Received %s, initiating graceful shutdown...\n", sig)
	}

	// Create shutdown context with timeout
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer shutdownCancel()

	// Graceful shutdown sequence
	fmt.Println("Stopping subscription manager...")
	if subMgr != nil {
		subMgr.Stop()
	}

	fmt.Println("Stopping box manager...")
	if err := boxMgr.Close(); err != nil {
		fmt.Printf("Error closing box manager: %v\n", err)
	}

	// Wait for connections to drain
	fmt.Println("Waiting for connections to drain...")
	select {
	case <-time.After(2 * time.Second):
		fmt.Println("Graceful shutdown completed")
	case <-shutdownCtx.Done():
		fmt.Println("Shutdown timeout exceeded, forcing exit")
	}

	return nil
}
