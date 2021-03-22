package main

import (
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"sync"
	"time"

	"github.com/judwhite/go-svc"
	"github.com/sirupsen/logrus"
	"github.com/takama/daemon"

	"github.com/vxcontrol/vxagent/mmodule"
	"github.com/vxcontrol/vxcommon/utils"
)

const (
	name        = "vxagent"
	description = "VXAgent service to the OS control"
)

// PackageVer is semantic version of vxagent
var PackageVer string

// PackageRev is revision of vxagent build
var PackageRev string

// Agent implements daemon structure
type Agent struct {
	debug   bool
	service bool
	logDir  string
	agentID string
	connect string
	command string
	module  *mmodule.MainModule
	wg      sync.WaitGroup
	svc     daemon.Daemon
}

// Init for preparing agent main module struct
func (a *Agent) Init(env svc.Environment) (err error) {
	utils.RemoveUnusedTempDir()
	a.module = mmodule.New(a.connect, a.agentID)
	if a.module == nil {
		err = fmt.Errorf("failed to create new main module")
		logrus.WithError(err).Error("failed to initialize")
		return
	}

	return
}

// Start logic of agent main module
func (a *Agent) Start() (err error) {
	logrus.Info("vxagent is starting...")
	a.wg.Add(1)
	go func() {
		defer a.wg.Done()
		err = a.module.Start()
	}()

	// Wait a little time to catch error on start
	time.Sleep(time.Second)
	logrus.Info("vxagent started")

	return
}

// Stop logic of agent main module
func (a *Agent) Stop() (err error) {
	logrus.Info("vxagent is stopping...")
	if err = a.module.Stop(); err != nil {
		return
	}
	logrus.Info("vxagent is waiting of modules release...")
	a.wg.Wait()
	logrus.Info("vxagent stopped")

	return
}

// Manage by daemon commands or run the daemon
func (a *Agent) Manage() (string, error) {
	switch a.command {
	case "install":
		opts := []string{
			"-service",
			"-agent", a.agentID,
			"-connect", a.connect,
			"-logdir", a.logDir,
		}
		if a.debug {
			opts = append(opts, "-debug")
		}
		return a.svc.Install(opts...)
	case "uninstall":
		return a.svc.Remove()
	case "start":
		return a.svc.Start()
	case "stop":
		return a.svc.Stop()
	case "status":
		return a.svc.Status()
	}

	if err := svc.Run(a); err != nil {
		logrus.WithError(err).Error("vxagent executing failed")
		return "vxagent running failed", err
	}

	logrus.Info("vxagent exited normaly")
	return "vxagent exited normaly", nil
}

func main() {
	var agent Agent
	var version bool
	flag.StringVar(&agent.connect, "connect", "ws://localhost:8080", "Connection string")
	flag.StringVar(&agent.agentID, "agent", "testid", "Agent ID for connection to server")
	flag.StringVar(&agent.command, "command", "", `Command to service control (not required):
  install - install the service to the system
  uninstall - uninstall the service from the system
  start - start the service
  stop - stop the service
  status - status of the service`)
	flag.StringVar(&agent.logDir, "logdir", "", "System option to define log directory to vxagent")
	flag.BoolVar(&agent.debug, "debug", false, "System option to run vxagent in debug mode")
	flag.BoolVar(&agent.service, "service", false, "System option to run vxagent as a service")
	flag.BoolVar(&version, "version", false, "Print current version of vxagent and exit")
	flag.Parse()

	if version {
		fmt.Printf("vxagent version is ")
		if PackageVer != "" {
			fmt.Printf("%s", PackageVer)
		} else {
			fmt.Printf("develop")
		}
		if PackageRev != "" {
			fmt.Printf("-%s\n", PackageRev)
		} else {
			fmt.Printf("\n")
		}

		os.Exit(0)
	}

	switch agent.command {
	case "install":
	case "uninstall":
	case "start":
	case "stop":
	case "status":
	case "":
	default:
		fmt.Println("invalid value of 'command' argument: ", agent.command)
		flag.PrintDefaults()
		os.Exit(1)
	}

	if os.Getenv("CONNECT") != "" {
		agent.connect = os.Getenv("CONNECT")
	}
	if os.Getenv("AGENT_ID") != "" {
		agent.agentID = os.Getenv("AGENT_ID")
	}
	if os.Getenv("LOG_DIR") != "" {
		agent.logDir = os.Getenv("LOG_DIR")
	}
	if os.Getenv("DEBUG") != "" {
		agent.debug = true
	}

	if agent.logDir == "" {
		agent.logDir = filepath.Dir(os.Args[0])
	}
	logDir, err := filepath.Abs(agent.logDir)
	if err != nil {
		fmt.Println("invalid value of 'logdir' argument: ", agent.logDir)
		os.Exit(1)
	} else {
		agent.logDir = logDir
	}

	if agent.debug {
		logrus.SetLevel(logrus.DebugLevel)
	} else {
		logrus.SetLevel(logrus.InfoLevel)
	}

	logPath := filepath.Join(agent.logDir, "agent.log")
	logFile, err := os.OpenFile(logPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		fmt.Println("failed to open log file: ", logPath)
		os.Exit(1)
	}
	if agent.service {
		logrus.SetOutput(logFile)
	} else {
		logrus.SetOutput(io.MultiWriter(os.Stdout, logFile))
	}

	kind := daemon.SystemDaemon
	var dependencies []string
	switch runtime.GOOS {
	case "linux":
		dependencies = []string{"multi-user.target", "sockets.target"}
	case "windows":
		dependencies = []string{"tcpip"}
	case "darwin":
		if os.Geteuid() == 0 {
			kind = daemon.GlobalDaemon
		} else {
			kind = daemon.UserAgent
		}
	default:
		logrus.Error("unsupported OS type")
		os.Exit(1)
	}

	agent.svc, err = daemon.New(name, description, kind, dependencies...)
	if err != nil {
		logrus.WithError(err).Error("vxagent service creating failed")
		os.Exit(1)
	}

	status, err := agent.Manage()
	if err != nil {
		fmt.Println(status, "\n", err.Error())
		os.Exit(1)
	}
	fmt.Println(status)
}
