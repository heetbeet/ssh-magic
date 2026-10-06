package main

import (
	"bufio"
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"encoding/pem"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"os/exec"
	"os/signal"
	"os/user"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"sync"
	"time"
	"unicode"

	"github.com/gofrs/flock"
	"github.com/jpillora/sshd-lite/sshd"
	"github.com/jpillora/sshd-lite/xssh"
	"github.com/pkg/sftp"
	"github.com/psanford/wormhole-william/wormhole"
	"golang.org/x/crypto/ssh"
	"golang.org/x/term"
)

const version = "0.1.1"
const lifetime = 2 * time.Hour

var endpointPattern = regexp.MustCompile(`^[a-f0-9]{64}$`)
var codePattern = regexp.MustCompile(`^[0-9]+(?:-[a-z]+){4}$`)

type invitation struct {
	Schema    string    `json:"schema"`
	SessionID string    `json:"session_id"`
	Endpoint  string    `json:"endpoint_id"`
	HostKey   string    `json:"ssh_host_key"`
	ClientKey string    `json:"ssh_client_private_key"`
	OS        string    `json:"host_os"`
	Host      string    `json:"host_name"`
	User      string    `json:"process_user"`
	Privilege string    `json:"privilege"`
	Expires   time.Time `json:"expires_at"`
	CodeHash  string    `json:"code_hash,omitempty"`
}
type hostStatus struct {
	Code    string    `json:"code,omitempty"`
	Phase   string    `json:"phase"`
	Expires time.Time `json:"expires_at"`
}

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), interruptSignals()...)
	defer stop()
	code, err := run(ctx, os.Args[1:])
	if err != nil {
		fmt.Fprintln(os.Stderr, "wh:", err)
		if code == 0 {
			code = 1
		}
	}
	os.Exit(code)
}
func run(ctx context.Context, a []string) (int, error) {
	if len(a) == 2 && a[0] == "proxy" {
		if !endpointPattern.MatchString(a[1]) {
			return 1, errors.New("invalid endpoint")
		}
		if e := containHost(); e != nil {
			return 1, e
		}
		bin, e := sidecar()
		if e != nil {
			return 1, e
		}
		cmd := exec.CommandContext(ctx, bin, "proxy", a[1])
		prepareChild(cmd)
		cmd.Stdin = os.Stdin
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		return 0, cmd.Run()
	}
	if len(a) == 0 || a[0] == "--help" || a[0] == "help" {
		fmt.Println(`SSH Wormhole ` + version + `
wh open [--admin]           Offer temporary access; keep this terminal open
wh connect [CODE]          Redeem the four-word code and save connection 'help'
wh exec help -- COMMAND    Run a command using the remote shell
wh put help LOCAL REMOTE   Upload a file
wh get help REMOTE LOCAL   Download a file
wh close help              Revoke remote access and erase local credentials
wh forget help             Erase local credentials without closing the host
wh status                  Show local sessions
wh remove                  Remove local credentials and cached downloads
wh version                 Print version
Ctrl+C or closing the host terminal ends access. Pairing expires in 10 minutes;
access expires two hours after open. No service, account, or firewall changes.`)
		return 0, nil
	}
	if a[0] == "version" {
		fmt.Println(version)
		return 0, nil
	}
	if a[0] == "open" && len(a) > 2 {
		return 1, errors.New("usage: wh open [--admin]")
	}
	if a[0] == "open" && len(a) == 2 && a[1] != "--admin" {
		return 1, errors.New("usage: wh open [--admin]")
	}
	if a[0] == "open" {
		admin := len(a) == 2
		if admin && !elevated() {
			return 0, elevate()
		}
		if !admin && elevated() {
			return 1, errors.New("elevated terminal: use open --admin explicitly, or open a normal terminal")
		}
	}
	root, err := dataRoot()
	if err != nil {
		return 1, err
	}
	if err = privateDir(root); err != nil {
		return 1, err
	}
	switch a[0] {
	case "open":
		return 0, openHost(ctx, root)
	case "connect":
		if e := containHost(); e != nil {
			return 1, e
		}
		if len(a) > 2 {
			return 1, errors.New("usage: wh connect [CODE]")
		}
		code := ""
		if len(a) == 2 {
			code = a[1]
		} else {
			fmt.Fprint(os.Stderr, "Code: ")
			if term.IsTerminal(int(os.Stdin.Fd())) {
				b, e := term.ReadPassword(int(os.Stdin.Fd()))
				fmt.Fprintln(os.Stderr)
				if e != nil {
					return 1, e
				}
				code = string(b)
			} else {
				code, _ = bufio.NewReader(os.Stdin).ReadString('\n')
			}
		}
		return 0, connect(ctx, root, strings.TrimSpace(code))
	case "status":
		for _, name := range []string{"host.json", "help/session.json"} {
			var v any
			if b, e := os.ReadFile(filepath.Join(root, name)); e == nil {
				if name == "help/session.json" {
					var d invitation
					if json.Unmarshal(b, &d) == nil {
						fmt.Printf("help: %s, %s, %s, expires %s\n", clean(d.Host), clean(d.User), d.Privilege, d.Expires.Format(time.RFC3339))
						continue
					}
				}
				if json.Unmarshal(b, &v) == nil {
					fmt.Printf("%s: %s\n", name, b)
				}
			}
		}
		fmt.Println("Cache:", root)
		return 0, nil
	case "remove":
		if _, e := loadInvitation(root); e == nil {
			return 1, errors.New("close help first, or use forget help to explicitly leave remote access open")
		}
		lock := flock.New(filepath.Join(root, "host.lock"))
		ok, e := lock.TryLock()
		if e != nil {
			return 1, e
		}
		if !ok {
			return 1, errors.New("close the local host terminal before removing")
		}
		lock.Unlock()
		return 0, removeProduct(root)
	}
	if len(a) < 2 || a[1] != "help" {
		return 1, errors.New("expected connection name 'help'; run wh --help")
	}
	if a[0] != "forget" {
		if e := containHost(); e != nil {
			return 1, e
		}
	}
	if a[0] == "forget" {
		if len(a) != 2 {
			return 1, errors.New("usage: wh forget help")
		}
		return 0, os.RemoveAll(filepath.Join(root, "help"))
	}
	d, e := loadInvitation(root)
	if e != nil {
		return 1, e
	}
	if a[0] == "exec" {
		if len(a) != 4 || a[2] != "--" {
			return 1, errors.New("usage: wh exec help -- 'remote command'")
		}
	}
	if (a[0] == "put" || a[0] == "get") && len(a) != 4 {
		return 1, errors.New("usage: wh put|get help SOURCE DESTINATION")
	}
	if a[0] != "exec" && a[0] != "put" && a[0] != "get" && a[0] != "close" {
		return 1, errors.New("unknown command; run wh --help")
	}
	c, e := dial(ctx, d)
	if e != nil {
		return 1, e
	}
	defer c.Close()
	switch a[0] {
	case "exec":
		s, e := c.NewSession()
		if e != nil {
			return 1, e
		}
		defer s.Close()
		s.Stdin = os.Stdin
		s.Stdout = os.Stdout
		s.Stderr = os.Stderr
		e = s.Run(a[3])
		var exit *ssh.ExitError
		if errors.As(e, &exit) {
			return exit.ExitStatus(), nil
		}
		return 0, e
	case "put", "get":
		return 0, transfer(c, a[0], a[2], a[3], d.OS)
	case "close":
		ok, _, e := c.SendRequest("close@ssh-wormhole", true, nil)
		if e != nil {
			return 1, fmt.Errorf("close unconfirmed, credentials retained: %w", e)
		}
		if !ok {
			return 1, errors.New("host refused closure")
		}
		c.Close()
		if e = os.RemoveAll(filepath.Join(root, "help")); e != nil {
			return 1, e
		}
		fmt.Println("Host acknowledged revocation. Local credentials erased.")
		return 0, nil
	}
	return 0, nil
}
func pairClient() *wormhole.Client {
	return &wormhole.Client{AppID: "io.github.heetbeet.ssh-wormhole/v1", PassPhraseComponentLength: 4, RendezvousURL: "wss://relay.magic-wormhole.io/v1"}
}
func key() (ssh.Signer, []byte, error) {
	_, priv, e := ed25519.GenerateKey(rand.Reader)
	if e != nil {
		return nil, nil, e
	}
	s, e := ssh.NewSignerFromKey(priv)
	if e != nil {
		return nil, nil, e
	}
	block, e := ssh.MarshalPrivateKey(priv, "")
	if e != nil {
		return nil, nil, e
	}
	return s, pem.EncodeToMemory(block), nil
}
func clean(s string) string {
	return strings.Map(func(r rune) rune {
		if unicode.IsControl(r) {
			return -1
		}
		return r
	}, s)
}
func save(path string, v any) error {
	b, e := json.MarshalIndent(v, "", "  ")
	if e != nil {
		return e
	}
	f, e := os.OpenFile(path+".new", os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if e != nil {
		return e
	}
	defer os.Remove(path + ".new")
	if _, e = f.Write(b); e != nil {
		f.Close()
		return e
	}
	if e = f.Sync(); e != nil {
		f.Close()
		return e
	}
	if e = f.Close(); e != nil {
		return e
	}
	return os.Rename(path+".new", path)
}
func openHost(parent context.Context, root string) error {
	lock := flock.New(filepath.Join(root, "host.lock"))
	ok, e := lock.TryLock()
	if e != nil {
		return e
	}
	if !ok {
		b, e := os.ReadFile(filepath.Join(root, "host.json"))
		if e == nil {
			fmt.Println(string(b))
			fmt.Println("Already open. Keep the original terminal open.")
			return nil
		}
		return errors.New("a host is starting; run this command again shortly")
	}
	defer lock.Unlock()
	os.Remove(filepath.Join(root, "host.json.new"))
	defer os.Remove(filepath.Join(root, "host.json"))
	if e = containHost(); e != nil {
		return e
	}
	ctx, cancel := context.WithTimeout(parent, lifetime)
	defer cancel()
	expiry := time.Now().Add(lifetime).UTC()
	go func() {
		t := time.NewTicker(time.Second)
		defer t.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-t.C:
				if !time.Now().Before(expiry) {
					cancel()
					return
				}
			}
		}
	}()
	host, hpem, e := key()
	if e != nil {
		return e
	}
	client, cpem, e := key()
	if e != nil {
		return e
	}
	home, e := os.UserHomeDir()
	if e != nil {
		return e
	}
	listener, e := net.Listen("tcp", "127.0.0.1:0")
	if e != nil {
		return e
	}
	defer listener.Close()
	authenticated := make(chan struct{}, 1)
	server, e := sshd.NewServer(sshd.Config{KeyBytes: hpem, AuthKeys: []ssh.PublicKey{client.PublicKey()}, SFTP: true, WorkDir: home, NoClientEnv: true, LogQuiet: true, HandshakeTimeout: 10 * time.Second, MaxPendingHandshakes: 16, Attach: commandHandler,
		ConnectionHandler: func(context.Context, *ssh.ServerConn) {
			select {
			case authenticated <- struct{}{}:
			default:
			}
		},
		GlobalRequestHandlers: map[string]xssh.GlobalRequestHandler{"close@ssh-wormhole": func(c xssh.Conn, r *xssh.Request) error {
			e := r.Reply(true, nil)
			go func() { time.Sleep(250 * time.Millisecond); cancel() }()
			return e
		}}})
	if e != nil {
		return e
	}
	serverDone := make(chan error, 1)
	go func() { serverDone <- server.StartWithContext(ctx, listener) }()
	defer func() { cancel(); <-serverDone }()
	binary, e := sidecar()
	if e != nil {
		return e
	}
	cmd := exec.CommandContext(ctx, binary, "server", "--ssh-port", fmt.Sprint(listener.Addr().(*net.TCPAddr).Port))
	prepareChild(cmd)
	out, e := cmd.StdoutPipe()
	if e != nil {
		return e
	}
	cmd.Stderr = os.Stderr
	if e = cmd.Start(); e != nil {
		return e
	}
	done := make(chan error, 1)
	processExited := make(chan struct{})
	go func() { done <- cmd.Wait(); close(processExited) }()
	defer func() { cmd.Process.Kill(); <-done }()
	ids := make(chan string, 1)
	go func() {
		sc := bufio.NewScanner(out)
		for sc.Scan() {
			for _, word := range strings.Fields(sc.Text()) {
				if i := strings.LastIndex(word, "@"); i >= 0 {
					end := word[i+1:]
					if endpointPattern.MatchString(end) {
						select {
						case ids <- end:
						default:
						}
					}
				}
			}
		}
	}()
	var endpoint string
	select {
	case endpoint = <-ids:
	case <-processExited:
		return errors.New("Iroh exited before becoming ready")
	case <-ctx.Done():
		return ctx.Err()
	case <-time.After(40 * time.Second):
		return errors.New("Iroh relay startup timed out")
	}
	sid := make([]byte, 16)
	if _, e = rand.Read(sid); e != nil {
		return e
	}
	hostname, _ := os.Hostname()
	u, _ := user.Current()
	username := ""
	if u != nil {
		username = u.Username
	}
	privilege := "user"
	if elevated() {
		privilege = "admin"
	}
	d := invitation{Schema: "wh/1", SessionID: hex.EncodeToString(sid), Endpoint: endpoint, HostKey: strings.TrimSpace(string(ssh.MarshalAuthorizedKey(host.PublicKey()))), ClientKey: string(cpem), OS: runtime.GOOS, Host: clean(hostname), User: clean(username), Privilege: privilege, Expires: expiry}
	payload, e := json.Marshal(d)
	if e != nil {
		return e
	}
	pairCtx, pairCancel := context.WithTimeout(ctx, 10*time.Minute)
	defer pairCancel()
	code, result, e := pairClient().SendText(pairCtx, string(payload))
	if e != nil {
		return fmt.Errorf("pairing relay: %w", e)
	}
	status := hostStatus{Code: code, Phase: "waiting", Expires: expiry}
	if e = save(filepath.Join(root, "host.json"), status); e != nil {
		return e
	}
	fmt.Printf("\nCODE: %s\n%s on %s (%s access)\nKeep this terminal open. Ctrl+C ends access. Expires %s\n", code, clean(username), clean(hostname), privilege, expiry.Format(time.RFC3339))
	select {
	case r := <-result:
		if r.Error != nil {
			if ctx.Err() != nil {
				return nil
			}
			return fmt.Errorf("pairing failed: %w", r.Error)
		}
		if !r.OK {
			return errors.New("pairing was not accepted")
		}
	case <-ctx.Done():
		return nil
	case <-processExited:
		return errors.New("Iroh stopped while waiting for pairing")
	}
	d.ClientKey = ""
	for i := range cpem {
		cpem[i] = 0
	}
	payload = nil
	status.Code = ""
	status.Phase = "paired"
	if e = save(filepath.Join(root, "host.json"), status); e != nil {
		return e
	}
	fmt.Println("Code redeemed. Waiting for SSH authentication...")
	select {
	case <-authenticated:
		fmt.Println("SSH authenticated. Assistance is active.")
	case <-time.After(2 * time.Minute):
		return errors.New("no SSH authentication after pairing; access closed")
	case <-ctx.Done():
		return nil
	case <-processExited:
		return errors.New("Iroh stopped before SSH authentication")
	}
	select {
	case <-ctx.Done():
	case <-processExited:
		return errors.New("Iroh stopped; access closed")
	}
	fmt.Println("Access closed.")
	return nil
}
func validate(d invitation) error {
	if d.Schema != "wh/1" || len(d.SessionID) != 32 || !endpointPattern.MatchString(d.Endpoint) || (d.OS != "windows" && d.OS != "linux") || (d.Privilege != "user" && d.Privilege != "admin") {
		return errors.New("invalid invitation")
	}
	if _, e := hex.DecodeString(d.SessionID); e != nil {
		return errors.New("invalid session ID")
	}
	if !time.Now().Before(d.Expires) || d.Expires.After(time.Now().Add(lifetime+time.Minute)) {
		return errors.New("invitation expired or invalid expiry")
	}
	pk, _, _, rest, e := ssh.ParseAuthorizedKey([]byte(d.HostKey))
	if e != nil || len(rest) != 0 || pk.Type() != ssh.KeyAlgoED25519 {
		return errors.New("invalid host key")
	}
	sk, e := ssh.ParsePrivateKey([]byte(d.ClientKey))
	if e != nil || sk.PublicKey().Type() != ssh.KeyAlgoED25519 {
		return errors.New("invalid client key")
	}
	return nil
}
func loadInvitation(root string) (invitation, error) {
	var d invitation
	b, e := os.ReadFile(filepath.Join(root, "help", "session.json"))
	if e != nil {
		return d, errors.New("no saved connection; run wh connect CODE")
	}
	if len(b) > 8192 {
		return d, errors.New("saved invitation too large")
	}
	if e = json.Unmarshal(b, &d); e != nil {
		return d, e
	}
	if !time.Now().Before(d.Expires) {
		os.RemoveAll(filepath.Join(root, "help"))
		return d, errors.New("connection expired; local credentials erased")
	}
	return d, validate(d)
}
func connect(ctx context.Context, root, code string) error {
	if !codePattern.MatchString(code) {
		return errors.New("expected numeric prefix followed by four words")
	}
	sum := sha256.Sum256([]byte(code))
	hash := hex.EncodeToString(sum[:])
	lock := flock.New(filepath.Join(root, "connect.lock"))
	ok, e := lock.TryLock()
	if e != nil {
		return e
	}
	if !ok {
		return errors.New("another connector is pairing; wait for it")
	}
	defer lock.Unlock()
	d, e := loadInvitation(root)
	if e == nil {
		if d.CodeHash != hash {
			return errors.New("connection 'help' already exists; close it first, or explicitly forget it")
		}
		c, e := dial(ctx, d)
		if e != nil {
			return e
		}
		c.Close()
		fmt.Println("Reusing saved connection help.")
		return nil
	}
	if _, e = os.Stat(filepath.Join(root, "help", "session.json")); e == nil {
		return errors.New("invalid saved credentials; use wh forget help before pairing again")
	}
	pairCtx, cancel := context.WithTimeout(ctx, 2*time.Minute)
	defer cancel()
	msg, e := pairClient().Receive(pairCtx, code)
	if e != nil {
		return fmt.Errorf("pairing: %w", e)
	}
	if msg.Type != wormhole.TransferText {
		msg.Reject()
		return errors.New("expected an SSH invitation")
	}
	b, e := io.ReadAll(io.LimitReader(msg, 8193))
	if e != nil {
		return e
	}
	if len(b) > 8192 {
		return errors.New("invitation too large")
	}
	dec := json.NewDecoder(strings.NewReader(string(b)))
	dec.DisallowUnknownFields()
	if e = dec.Decode(&d); e != nil {
		return e
	}
	if e = dec.Decode(new(any)); e != io.EOF {
		return errors.New("extra data in invitation")
	}
	if e = validate(d); e != nil {
		return e
	}
	d.CodeHash = hash
	dir := filepath.Join(root, "help")
	if e = privateDir(dir); e != nil {
		return e
	}
	os.Remove(filepath.Join(dir, "session.json.new"))
	if e = save(filepath.Join(dir, "session.json"), d); e != nil {
		return e
	}
	if e = exportSSH(dir, d); e != nil {
		return e
	}
	c, e := dial(ctx, d)
	if e != nil {
		return fmt.Errorf("invitation saved; retry connect with the same code: %w", e)
	}
	defer c.Close()
	exe, _ := os.Executable()
	fmt.Printf("Connected: %s / %s / %s\nConnection: help\nExecutable: %s\nSSH config: %s\n", clean(d.Host), clean(d.User), d.Privilege, exe, filepath.Join(dir, "ssh_config"))
	return nil
}
func exportSSH(dir string, d invitation) error {
	if e := os.WriteFile(filepath.Join(dir, "identity"), []byte(d.ClientKey), 0600); e != nil {
		return e
	}
	if e := os.WriteFile(filepath.Join(dir, "known_hosts"), []byte(d.Endpoint+" "+d.HostKey+"\n"), 0600); e != nil {
		return e
	}
	exe, e := os.Executable()
	if e != nil {
		return e
	}
	q := func(s string) string { return `"` + strings.ReplaceAll(filepath.ToSlash(s), `"`, `\"`) + `"` }
	config := fmt.Sprintf("Host help\n HostName %s\n User help\n ProxyCommand %s proxy %s\n IdentityFile %s\n UserKnownHostsFile %s\n GlobalKnownHostsFile none\n StrictHostKeyChecking yes\n IdentitiesOnly yes\n IdentityAgent none\n ForwardAgent no\n PasswordAuthentication no\n KbdInteractiveAuthentication no\n ConnectTimeout 40\n ServerAliveInterval 15\n ServerAliveCountMax 3\n", d.Endpoint, q(exe), d.Endpoint, q(filepath.Join(dir, "identity")), q(filepath.Join(dir, "known_hosts")))
	return os.WriteFile(filepath.Join(dir, "ssh_config"), []byte(config), 0600)
}
func dial(ctx context.Context, d invitation) (*ssh.Client, error) {
	if e := validate(d); e != nil {
		return nil, e
	}
	sk, e := ssh.ParsePrivateKey([]byte(d.ClientKey))
	if e != nil {
		return nil, e
	}
	pk, _, _, _, e := ssh.ParseAuthorizedKey([]byte(d.HostKey))
	if e != nil {
		return nil, e
	}
	bin, e := sidecar()
	if e != nil {
		return nil, e
	}
	p, e := startProxy(ctx, bin, d.Endpoint)
	if e != nil {
		return nil, e
	}
	p.SetDeadline(time.Now().Add(40 * time.Second))
	conn, ch, req, e := ssh.NewClientConn(p, d.Endpoint, &ssh.ClientConfig{User: "help", Auth: []ssh.AuthMethod{ssh.PublicKeys(sk)}, HostKeyCallback: ssh.FixedHostKey(pk), Timeout: 40 * time.Second})
	if e != nil {
		p.Close()
		return nil, e
	}
	p.SetDeadline(time.Time{})
	c := ssh.NewClient(conn, ch, req)
	go func() {
		select {
		case <-ctx.Done():
			c.Close()
		case <-p.done:
		}
	}()
	// A real-time expiry timer is paired with a wall-clock check to cover system suspend.
	go func() {
		t := time.NewTicker(time.Second)
		defer t.Stop()
		for {
			select {
			case <-p.done:
				return
			case <-t.C:
				if !time.Now().Before(d.Expires) {
					c.Close()
					return
				}
			}
		}
	}()
	return c, nil
}

type proxyConn struct {
	cmd   *exec.Cmd
	in    io.WriteCloser
	out   io.ReadCloser
	done  chan struct{}
	once  sync.Once
	mu    sync.Mutex
	timer *time.Timer
}

func startProxy(ctx context.Context, bin, id string) (*proxyConn, error) {
	cmd := exec.CommandContext(ctx, bin, "proxy", id)
	prepareChild(cmd)
	in, e := cmd.StdinPipe()
	if e != nil {
		return nil, e
	}
	out, e := cmd.StdoutPipe()
	if e != nil {
		in.Close()
		return nil, e
	}
	cmd.Stderr = os.Stderr
	if e = cmd.Start(); e != nil {
		in.Close()
		out.Close()
		return nil, e
	}
	p := &proxyConn{cmd: cmd, in: in, out: out, done: make(chan struct{})}
	go func() { cmd.Wait(); close(p.done) }()
	return p, nil
}
func (p *proxyConn) Read(b []byte) (int, error)  { return p.out.Read(b) }
func (p *proxyConn) Write(b []byte) (int, error) { return p.in.Write(b) }
func (p *proxyConn) Close() error {
	p.once.Do(func() {
		p.mu.Lock()
		if p.timer != nil {
			p.timer.Stop()
		}
		p.mu.Unlock()
		p.in.Close()
		p.out.Close()
		p.cmd.Process.Kill()
		<-p.done
	})
	return nil
}

type pipeAddr string

func (a pipeAddr) Network() string        { return "iroh" }
func (a pipeAddr) String() string         { return string(a) }
func (p *proxyConn) LocalAddr() net.Addr  { return pipeAddr("local") }
func (p *proxyConn) RemoteAddr() net.Addr { return pipeAddr("remote") }
func (p *proxyConn) SetDeadline(t time.Time) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.timer != nil {
		p.timer.Stop()
	}
	if !t.IsZero() {
		p.timer = time.AfterFunc(time.Until(t), func() { p.Close() })
	}
	return nil
}
func (p *proxyConn) SetReadDeadline(t time.Time) error  { return p.SetDeadline(t) }
func (p *proxyConn) SetWriteDeadline(t time.Time) error { return p.SetDeadline(t) }
func remotePath(s, hostOS string) string {
	if hostOS == "windows" {
		s = strings.ReplaceAll(s, `\`, "/")
		if len(s) >= 3 && s[1] == ':' && s[2] == '/' {
			s = "/" + s
		}
	}
	return s
}
func transfer(c *ssh.Client, direction, src, dst, hostOS string) error {
	sc, e := sftp.NewClient(c)
	if e != nil {
		return e
	}
	defer sc.Close()
	if direction == "put" {
		dst = remotePath(dst, hostOS)
		f, e := os.Open(src)
		if e != nil {
			return e
		}
		defer f.Close()
		nonce := make([]byte, 8)
		if _, e = rand.Read(nonce); e != nil {
			return e
		}
		tmp := dst + ".wh-" + hex.EncodeToString(nonce)
		remote, e := sc.OpenFile(tmp, os.O_WRONLY|os.O_CREATE|os.O_EXCL)
		if e != nil {
			return e
		}
		defer sc.Remove(tmp)
		_, e = io.Copy(remote, f)
		ce := remote.Close()
		if e != nil {
			return e
		}
		if ce != nil {
			return ce
		}
		if _, ok := sc.HasExtension("posix-rename@openssh.com"); ok {
			return sc.PosixRename(tmp, dst)
		}
		return sc.Rename(tmp, dst)
	}
	f, e := sc.Open(remotePath(src, hostOS))
	if e != nil {
		return e
	}
	defer f.Close()
	tmp, e := os.CreateTemp(filepath.Dir(dst), ".wh-*")
	if e != nil {
		return e
	}
	defer os.Remove(tmp.Name())
	_, e = io.Copy(tmp, f)
	if e != nil {
		tmp.Close()
		return e
	}
	if e = tmp.Sync(); e != nil {
		tmp.Close()
		return e
	}
	if e = tmp.Close(); e != nil {
		return e
	}
	return os.Rename(tmp.Name(), dst)
}
