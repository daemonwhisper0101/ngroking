// vim:set sw=2 sts=2:
package ngroking

import (
  "bufio"
  "io/ioutil"
  "log"
  "math/rand"
  "net/http"
  "os"
  "os/exec"
  "path/filepath"
  "strings"
  "syscall"
  "time"
)

type Conn interface {
  Start()
  Stop()
  URL() string
  LiveTime() time.Duration
  CurrentProxy() string
  Destroy()
}

type ngrok struct {
  bin string
  addr string
  proxies []string
  curr string
  tmpdir string
  process *os.Process
  wait chan struct{}
  start time.Time
  webaddr string
  public_url string
  logger *log.Logger
}

func (n *ngrok)Start() {
  if n.process != nil {
    return
  }

  start := make(chan struct{})
  go func() {
    var cmd *exec.Cmd
    n.logger.Println(n.proxies)
proxyselection:
    for _, proxy := range n.proxies {
      n.logger.Printf("trying proxy %s\n", proxy)
      n.curr = proxy
      cmd = exec.Command(n.bin, "start", "-config", "config.yml", "https")
      cmd.Dir = n.tmpdir
      cmd.SysProcAttr = &syscall.SysProcAttr{ Setpgid: true }
      sout, _ := cmd.StdoutPipe()
      cmd.Env = append(os.Environ(),
		       "http_proxy=" + proxy,
		       "https_proxy=" + proxy)
      err := cmd.Start()
      if err != nil {
	n.logger.Printf("cmd.Start: %v\n", err)
	continue
      }
      n.process = cmd.Process
      n.start = time.Now()
      //n.logger.Println(sin, sout, serr)
      // log checker
      estab := make(chan struct{})
      go func() {
	pid := n.process.Pid
/*
t=2018-08-15T07:49:28+0900 lvl=info msg="starting web service" obj=web addr=127.0.0.1:4040
t=2018-08-15T07:49:30+0900 lvl=info msg="tunnel session started" obj=tunSess
t=2018-08-15T07:49:30+0900 lvl=info msg="client session established" obj=csess id=69d425ab3fc5
t=2018-08-15T07:50:28+0900 lvl=info msg="all component stopped" obj=controller
*/
	s := bufio.NewScanner(sout)
logloop:
	for s.Scan() {
	  line := s.Text()
	  if strings.Index(line, "starting web service") != -1 {
	    n.logger.Printf("[%d] %s\n", pid, line)
	    a := strings.Split(line, "addr=")
	    n.webaddr = "http://" + a[1]
	    n.logger.Printf("[%d] web service @ %s\n", pid, n.webaddr)
	  } else if strings.Index(line, "client session established") != -1 {
	    n.logger.Printf("[%d] %s\n", pid, line)
	    estab <- struct{}{}
	  } else if strings.Index(line, "all component stopped") != -1 {
	    n.logger.Printf("[%d] %s\n", pid, line)
	    break logloop
	  }
	}
      }()
      // wait establisthed
      select {
      case <-estab: break proxyselection
      case <-time.After(5 * time.Second):
      }
      n.process.Kill()
      cmd.Wait()
      n.process = nil
      n.start = time.Time{}
    }
    //
    start <- struct{}{} // process started
    if n.process != nil {
      cmd.Wait()
      time.Sleep(time.Millisecond)
    }
    n.wait <- struct{}{}

    n.logger.Printf("[%d] done\n", n.process.Pid)
  }()
  <-start
  if n.process != nil {
    n.logger.Printf("[%d] start\n", n.process.Pid)
  } else {
    n.logger.Printf("unable to start\n")
  }
}

func (n *ngrok)Stop() {
  if n.process == nil {
    return
  }
  n.process.Signal(os.Interrupt)
  <-n.wait
  // reset
  n.public_url = ""
  n.webaddr = ""
  n.curr = ""
  n.process = nil
}

func (n *ngrok)URL() string {
  if n.public_url != "" {
    return n.public_url
  }
  if n.process == nil || n.webaddr == "" {
    return ""
  }
  // ngrok process must have webaddr
  resp, err := http.Get(n.webaddr + "/api/tunnels")
  if err != nil {
    n.logger.Printf("??? %v\n", err)
    return ""
  }
  defer resp.Body.Close()
  body, err := ioutil.ReadAll(resp.Body)
  if err != nil {
    n.logger.Printf("??? %v\n", err)
    return ""
  }
/*
  n.logger.Printf("tunnels: %s\n", string(body))
tunnels: {"tunnels":[{"name":"https","uri":"/api/tunnels/https","public_url":"https://8868f41c.ngrok.io","proto":"https","config":{"addr":"localhost:9999","inspect":false},"metrics":{"conns":{"count":0,"gauge":0,"rate1":0,"rate5":0,"rate15":0,"p50":0,"p90":0,"p95":0,"p99":0},"http":{"count":0,"rate1":0,"rate5":0,"rate15":0,"p50":0,"p90":0,"p95":0,"p99":0}}}],"uri":"/api/tunnels"}
*/
  json := string(body)
  if strings.Index(json, "public_url") == -1 {
    return ""
  }
  a0 := strings.Split(json, `"public_url":"`)
  a1 := strings.Split(a0[1], `","`)
  n.public_url = a1[0]
  n.logger.Printf("public_url: %s\n", n.public_url)
  return n.public_url
}

func (n *ngrok)LiveTime() time.Duration {
  if n.process == nil {
    return 0
  }
  return time.Now().Sub(n.start)
}

func (n *ngrok)CurrentProxy() string {
  if n.process == nil {
    return ""
  }
  return n.curr
}

func (n *ngrok)Destroy() {
  // make sure ngrok process stopped
  n.Stop()
  // remove tmpdir
  os.RemoveAll(n.tmpdir)
}

func New(bin, addr string, proxies []string, logger *log.Logger) Conn {
  absbin, err := filepath.Abs(bin)
  if err != nil {
    // error
    absbin = ""
    return nil
  }
  ps := make([]string, len(proxies))
  copy(ps, proxies)
  // shuffle
  for i := 0; i < len(ps); i++ {
    a := rand.Intn(len(ps))
    b := rand.Intn(len(ps))
    if a != b {
      ps[a], ps[b] = ps[b], ps[a]
    }
  }
  logger.Println(ps)

  // create tmpdir
  tmpdir, err := ioutil.TempDir("", "ngroking")
  if err != nil {
    return nil
  }
  // create config
  f, err := os.OpenFile(filepath.Join(tmpdir, "config.yml"), os.O_RDWR|os.O_CREATE, 0644)
  if err != nil {
    return nil
  }
  config := `log_level: info
log_format: logfmt
log: stdout
tunnels:
  https:
    proto: http
    addr: ` + addr + `
    inspect: false
    bind_tls: true
`
  f.Write([]byte(config))
  f.Close()

  return &ngrok{
    bin: absbin,
    addr: addr,
    proxies: ps,
    logger: logger,
    tmpdir: tmpdir,
    wait: make(chan struct{}),
  }
}
