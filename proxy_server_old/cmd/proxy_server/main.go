package main

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httputil"
	"net/url"
	"os"
	"os/signal"
	"path"
	"sync"
	"syscall"
	"time"

	"github.com/gin-gonic/gin"
	goutil "github.com/sebmak/go_util"
	pb "github.com/sebmak/proxy_server/pkg/proto"
	"golang.org/x/crypto/acme/autocert"
	"golang.org/x/sync/errgroup"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

var g errgroup.Group

type ProxyTarget struct {
	Host         string `json:"host,omitempty"`
	Scheme       string `json:"scheme,omitempty"`
	ForceSSL     bool   `json:"force_ssl,omitempty"`
	RequireAuth  bool   `json:"require_auth"`
	AuthResource string `json:"auth_resource"`
}

type ProxyTargets struct {
	Targets map[string]*ProxyTarget `json:"targets"`
	mu      sync.Mutex
}

func (p *ProxyTargets) GetTarget(key string) *ProxyTarget {
	p.mu.Lock()
	defer p.mu.Unlock()

	if t, ok := p.Targets[key]; ok {
		return t
	}

	return nil
}

var targets *ProxyTargets

func healthcheckHandler() *gin.Engine {
	e := gin.Default()
	e.Use(gin.Recovery())
	e.GET("/", func(c *gin.Context) {
		c.JSON(
			http.StatusOK,
			gin.H{
				"code": http.StatusOK,
			},
		)
	})

	return e
}

func getSessionToken(req *http.Request) (string, error) {
	cookie, err := req.Cookie(goutil.GetEnvVariable("SESSION_COOKIE_NAME", "snickerdoodle"))
	if err != nil {
		return "", err
	}

	return cookie.Value, nil
}

func isAuthenticated(req *http.Request, authResource string) bool {
	token, err := getSessionToken(req)

	if err != nil {
		goutil.Logger().Debugf("Error no session token: %s", err.Error())
		return false
	}

	conn, err := grpc.Dial(goutil.GetEnvVariable("AUTH_RPC", "localhost:2110"), grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		goutil.Logger().Debugf("Error connecting to auth server: %s", err.Error())
		return false
	}
	defer conn.Close()

	client := pb.NewAuthClient(conn)

	res, err := client.CheckAuth(req.Context(), &pb.CheckAuthRequest{
		Session:  token,
		Resource: authResource,
	})

	if err != nil {
		goutil.Logger().Debugf("Error checking auth: %s", err.Error())
		return false
	}

	goutil.Logger().Debugf("Auth found for %s: %v", token, res)

	return res.Authorized && res.Verified
}

// Serve a reverse proxy for a given url
func serveReverseProxy(target string, res http.ResponseWriter, req *http.Request) {
	logRequestPayload(target)
	// parse the url
	url, _ := url.Parse(target)

	// create the reverse proxy
	proxy := httputil.NewSingleHostReverseProxy(url)

	// Note that ServeHttp is non blocking and uses a go routine under the hood
	proxy.ServeHTTP(res, req)
}

// Log the typeform payload and redirect url
func logRequestPayload(proxyURL string) {
	goutil.Logger().Infof("Proxying to: %s\n", proxyURL)
}

func loadProxyTargets() error {
	servers, err := os.ReadFile(path.Join(goutil.GetEnvVariable("CONFIG_DIR", "/data"), "servers.json"))
	if err != nil {
		return err
	}

	targets = &ProxyTargets{}

	err = json.Unmarshal(servers, targets)
	if err != nil {
		return err
	}

	return nil
}
func redirectToSsl(w http.ResponseWriter, r *http.Request) {
	r.URL.Scheme = "https"
	r.URL.Host = r.Host

	if r.URL.Port() == "9080" {
		r.URL.Host = fmt.Sprintf("%s:9443", r.URL.Hostname())
	}

	goutil.Logger().Infof("Redirecting to %s", r.URL.String())

	http.Redirect(w, r, r.URL.String(), http.StatusMovedPermanently)
}

func getAuthRedirectUrl(req *http.Request) string {
	u, err := url.Parse(goutil.GetEnvVariable("AUTH_DOMAIN", "http://localhost:9090"))
	if err != nil {
		return ""
	}
	q := u.Query()
	q.Set("r", fmt.Sprintf("%s%s", req.Host, req.URL.String()))
	u.RawQuery = q.Encode()

	return u.String()
}

// Given a request send it to the appropriate url
func handleRequestAndRedirect(res http.ResponseWriter, req *http.Request) {
	reqUrl := url.URL{
		Host: req.Host,
	}

	target := targets.GetTarget(reqUrl.Hostname())

	if target == nil {
		goutil.ReturnGeneric(res, nil, http.StatusNotFound)
		return
	}

	goutil.Logger().Debugf("TLS: %v", req.TLS)

	if target.ForceSSL && req.TLS == nil {
		redirectToSsl(res, req)
		return
	}

	if target.RequireAuth && !isAuthenticated(req, target.AuthResource) {
		http.Redirect(res, req, getAuthRedirectUrl(req), http.StatusFound)
		return
	}

	// targ := target.Target
	proxyUrl := url.URL{
		Scheme: target.Scheme,
		Host:   target.Host,
	}
	serveReverseProxy(proxyUrl.String(), res, req)
}

// func insecureProxy(handler http.Handler) error {
// 	err := loadProxyTargets()
// 	if err != nil {
// 		goutil.Logger().Fatal(err)
// 	}

// 	http.HandleFunc("/", handleRequestAndRedirect)

// 	return http.ListenAndServe(":9080", handler)
// }

func main() {
	err := loadProxyTargets()
	if err != nil {
		goutil.Logger().Fatal(err)
	}

	SetupCloseHandler()

	mux := http.NewServeMux()
	mux.HandleFunc("/", handleRequestAndRedirect)

	certDir := path.Join(goutil.GetEnvVariable("CONFIG_DIR", "/data"), "certs")

	hostPolicy := func(ctx context.Context, host string) error {
		target := targets.GetTarget(host)

		if target != nil {
			return nil
		}

		return fmt.Errorf("acme/autocert: host %s is not allowed", host)
	}

	m := &autocert.Manager{
		Prompt:     autocert.AcceptTOS,
		HostPolicy: hostPolicy,
		Email:      goutil.GetEnvVariable("EMAIL", "sebastian.makinson@gmail.com"),
		Cache:      autocert.DirCache(certDir),
	}

	srv := &http.Server{
		Addr:    ":9443",
		Handler: mux,
		TLSConfig: &tls.Config{
			GetCertificate: m.GetCertificate,
		},
	}

	httpserv := http.NewServeMux()
	httpserv.HandleFunc("/", handleRequestAndRedirect)

	g.Go(func() error {
		return http.ListenAndServe(":9080", m.HTTPHandler(httpserv))
	})

	g.Go(func() error {
		return srv.ListenAndServeTLS("", "")
	})

	healthcheck := &http.Server{
		Addr:         ":9081",
		Handler:      healthcheckHandler(),
		ReadTimeout:  5 * time.Second,
		WriteTimeout: 10 * time.Second,
	}

	g.Go(func() error {
		return healthcheck.ListenAndServe()
	})

	if err := g.Wait(); err != nil {
		goutil.Logger().Fatal(err)
	}

}

// SetupCloseHandler creates a 'listener' on a new goroutine which will notify the
// program if it receives an interrupt from the OS. We then handle this by calling
// our clean up procedure and exiting the program.
func SetupCloseHandler() {
	c := make(chan os.Signal)
	signal.Notify(c, os.Interrupt, syscall.SIGTERM)
	go func() {
		<-c
		cleanup()

		os.Exit(0)
	}()
}

func cleanup() {
}
