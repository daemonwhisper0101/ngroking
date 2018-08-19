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

  "github.com/daemonwhisper0101/ngroking"
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

func main() {
  if len(os.Args) < 4 {
    os.Exit(1)
  }
  bin := os.Args[1]
  addr := os.Args[2]
  proxies := os.Args[3:]
  // create 4 connections
  ngrok := []ngroking.Conn{}
  mylog := &MyLogger{ logs: []string{} }
  logger := log.New(mylog, "", log.LstdFlags)
  //logger = log.New(os.Stdout, "", log.LstdFlags)
  for i := 0; i < 4; i++ {
    ngrok = append(ngrok, ngroking.New(bin, addr, proxies, logger))
  }
  // catch Ctrl-C
  quit := make(chan struct{})
  signalhandler(quit)

  // manage
loop:
  for {
    for i := 0; i < 4; i++ {
      // restart ngrok
      ngrok[i].Stop()
      time.Sleep(time.Second)
      ngrok[i].Start()
      for n := 0; n < 5; n++ {
	if ngrok[i].URL() != "" {
	  break
	}
	time.Sleep(time.Second)
      }
      fmt.Println(ngrok[i].URL())
      select {
      case <-quit: break loop
      //case <-time.After(time.Hour * 2):
      case <-time.After(time.Second * 30):
      }
    }
  }
  fmt.Println("Stopping")
  for i := 0; i < 4; i++ {
    ngrok[i].Stop()
  }
  for i := 0; i < 4; i++ {
    ngrok[i].Destroy()
  }
  // show logs
  for _, log := range mylog.logs {
    fmt.Println(log)
  }
}
