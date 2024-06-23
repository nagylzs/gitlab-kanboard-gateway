package main

import (
	"fmt"
	"github.com/jessevdk/go-flags"
	"github.com/nagylzs/gitlab-kanboard-gateway/assets"
	"github.com/nagylzs/gitlab-kanboard-gateway/internal/config"
	"github.com/nagylzs/gitlab-kanboard-gateway/internal/processor"
	"github.com/nagylzs/gitlab-kanboard-gateway/internal/signal"
	"github.com/nagylzs/gitlab-kanboard-gateway/internal/version"
	"github.com/nagylzs/gitlab-kanboard-gateway/internal/websrv"
	"log/slog"
	"os"
	"time"
)

var cfg *config.Config = nil

func main() {
	args := &config.GatewayOpts
	_, err := flags.ParseArgs(args, os.Args)
	if err != nil {
		println(err.Error())
		os.Exit(1)
	}

	if args.ShowVersion {
		version.PrintVersion()
		os.Exit(0)
	}

	if args.ShowInfo {
		repo := "https://github.com/nagylzs/gitlab-kanboard-gateway/"
		baseRef := fmt.Sprintf("%v-/blob/%v", repo, version.Commit)
		fmt.Println(fmt.Sprintf(`

gitlab-kanboard-gateway
=======================

A tool that receives webhook push events from GitLab and creates kanboard comments from them.
For details see %v

Version information
-------------------

Branch: %v
Commit: %v
Build Date: %v

Template arguments definitions:

* %v
* %v
* %v

Example config file:

%v

		`,
			repo,
			version.Branch, version.Commit, version.Built,
			baseRef+"/internal/processor/template_args.go",
			baseRef+"/internal/webhooks/pushevent.go",
			baseRef+"/internal/kanboard/rpc_type_task.go",
			assets.ConfigExampleYml,
		))
		os.Exit(0)
	}

	cfg, err = config.LoadConfig(args.ConfigFile)
	if err != nil {
		println(err.Error())
		os.Exit(1)
	}

	// Set loglevel
	var programLevel = new(slog.LevelVar)
	if args.Debug {
		programLevel.Set(slog.LevelDebug)
	} else if args.Verbose {
		programLevel.Set(slog.LevelInfo)
	} else {
		programLevel.Set(slog.LevelWarn)
	}
	h := slog.NewJSONHandler(os.Stderr, &slog.HandlerOptions{Level: programLevel})
	slog.SetDefault(slog.New(h))

	signal.SetupSignalHandler()

	go websrv.ServeHttp(cfg.Webhook)
	proc := processor.NewProcessor(&cfg.Kanboard, cfg.CommentTemplate)
	go proc.Run()

	for !signal.IsStopping() {
		time.Sleep(time.Second)
	}
	slog.Warn("Stopping...")
	time.Sleep(2 * time.Second)
}
