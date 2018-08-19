// vim:set sw=2 sts=2:
package main

import (
  "fmt"
  "log"
  "os"
  "os/signal"
  "strings"
  "syscall"
  "time"

  "github.com/daemonwhisper0101/ngroking/keeper"
)

type MyLogger struct {
  logs []string
}

func (l *MyLogger)Write(p []byte) (int, error) {
  log := strings.TrimSpace(string(p))
  l.logs = append(l.logs, log)
  return len(p), nil
}

func signalhandler(quit chan struct{}) {
  signal_chan := make(chan os.Signal)
  signal.Notify(signal_chan, syscall.SIGINT, syscall.SIGTERM)
  go func() {
    for {
      select {
      case <-signal_chan:
	fmt.Println("sig")
	quit <- struct{}{}
      }
    }
  }()
}

func run(bin, addr string, proxies []string) {
  logger := log.New(os.Stdout, "", log.LstdFlags)
  lifetime := time.Minute * 2
  k := keeper.New(4, lifetime, bin, addr, proxies, logger)
  k.Start()
  k.Stop()
}

func main() {
  if len(os.Args) < 4 {
    os.Exit(1)
  }
  bin := os.Args[1]
  addr := os.Args[2]
  proxies := os.Args[3:]
  lifetime := time.Minute * 2
  // create 4 connections
  mylog := &MyLogger{ logs: []string{} }
  logger := log.New(mylog, "", log.LstdFlags)
  //logger = log.New(os.Stdout, "", log.LstdFlags)
  k := keeper.New(4, lifetime, bin, addr, proxies, logger)
  // catch Ctrl-C
  quit := make(chan struct{})
  signalhandler(quit)

  k.Start()
  // manage
loop:
  for {
    for i := 0; i < 4; i++ {
      ngrok := k.GetInstance(i)
      if ngrok != nil {
	if ngrok.URL() != "" {
	  fmt.Printf("%s via %s\n", ngrok.URL(), ngrok.CurrentProxy())
	}
      }
    }
    select {
    case <-quit: break loop
    case <-time.After(time.Minute):
    }
  }
  fmt.Println("Stopping")
  k.Stop()
  k.Destroy()
  // show logs
  for _, log := range mylog.logs {
    fmt.Println(log)
  }
}
