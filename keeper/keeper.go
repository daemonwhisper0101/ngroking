// vim:set sw=2 sts=2:
package keeper

import (
  "log"
  "time"

  "github.com/daemonwhisper0101/ngroking"
)

type Keeper interface {
  Start()
  Stop()
  Destroy()
  GetInstance(i int) ngroking.Conn
}

type keeper struct {
  ngrok []ngroking.Conn
  //
  lifetime time.Duration
  bin, addr string
  proxies []string
  logger *log.Logger
  //
  running bool
  quit chan struct{}
  done chan struct{}
}

func (k *keeper)worker() {
  k.logger.Println("start worker")
  // wake up all instance
  for i, ngrok := range k.ngrok {
    k.logger.Printf("instance#%d start\n", i)
    ngrok.Start()
    time.Sleep(time.Second)
  }
  for k.running {
    for i, ngrok := range k.ngrok {
      live := ngrok.LiveTime()
      k.logger.Printf("instance#%d livetime %v\n", i, live)
      if live == 0 { // not running
	k.logger.Printf("instance#%d start\n", i)
	ngrok.Start()
      }
      if live > k.lifetime {
	k.logger.Printf("instance#%d restart\n", i)
	ngrok.Stop()
	time.Sleep(time.Second)
	ngrok.Start()
	break
      }
    }
    nr := time.Duration(len(k.ngrok))
    interval := k.lifetime / nr
    if interval < time.Second * 5 {
      interval = time.Second * 5
    }
    timeout := time.After(k.lifetime / nr)
    select {
    case <-k.quit:
    case <-timeout:
    }
  }
  // stop all instance
  for i, ngrok := range k.ngrok {
    k.logger.Printf("instance#%d stop\n", i)
    ngrok.Stop()
  }
  k.logger.Println("stop worker")
  k.done <- struct{}{}
}

func (k *keeper)Start() {
  if k.running {
    return
  }
  // start worker
  k.running = true
  go k.worker()
}

func (k *keeper)Stop() {
  if !k.running {
    return
  }
  k.running = false
  k.quit <- struct{}{}
  <-k.done
}

func (k *keeper)Destroy() {
  k.logger.Println("destroy all instance")
  for _, ngrok := range k.ngrok {
    ngrok.Destroy()
  }
}

func (k *keeper)GetInstance(i int) ngroking.Conn {
  if i < len(k.ngrok) {
    return k.ngrok[i]
  }
  return nil
}

func New(ninst int, lifetime time.Duration, bin, addr string, proxies []string, logger *log.Logger) Keeper {
  k := &keeper{
    ngrok: []ngroking.Conn{},
    lifetime: lifetime,
    bin: bin,
    addr: addr,
    proxies: proxies,
    logger: logger,
    running: false,
    quit: make(chan struct{}),
    done: make(chan struct{}),
  }
  for i := 0; i < ninst; i++ {
    ngrok := ngroking.New(bin, addr, proxies, logger)
    k.ngrok = append(k.ngrok, ngrok)
  }
  return k
}
