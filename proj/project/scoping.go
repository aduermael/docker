package project

import (
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/docker/docker/api"
	"github.com/docker/docker/client"
	proxyFakes "github.com/docker/engine-api-proxy/fakes"
	proxyPipeline "github.com/docker/engine-api-proxy/pipeline"
	"github.com/docker/engine-api-proxy/proxy"
)

// IsInProject indicates whether we are in the context of a project
func IsInProject() bool {
	return CurrentProject != nil
}

// StartInMemoryProxy ...
// "unix:///var/run/docker.sock"
func StartInMemoryProxy(proj Project, backendAddr string) (*proxy.Proxy, error) {
	if proj == nil {
		return nil, errors.New("can't create proxy outside of project")
	}
	fmt.Println("starting in-memory project proxy...")

	// creates a docker API client that will be used internally by the proxy
	dockerClient, err := client.NewClient(backendAddr, api.DefaultVersion, nil, nil) // TODO: gdevillele: review this
	if err != nil {
		return nil, err
	}

	// obtain proxy routes
	proxyRoutes := proxyPipeline.MiddlewareRoutes(dockerClient, func() proxyPipeline.Scoper {
		return proxyPipeline.NewProjectScoper(proj.Name(), proj.ID())
	})

	// construct proxy options struct
	proxyOpts := proxy.Options{
		Listen:      "",
		Backend:     backendAddr,
		SocketGroup: "",
		Routes:      proxyRoutes,
	}

	// create in-memory proxy
	p, err := proxy.NewInMemoryProxy(proxyOpts)
	if err != nil {
		return nil, err
	}

	// start proxy server goroutine
	go p.Start()

	// return proxy server
	return p, nil
}

// // StopInMemoryProxy ...
// func StopInMemoryProxy(proxy Proxy) {
// 	// TODO: close connections &stop proxy
// }

// NewScopedHttpClient ...
func NewScopedHttpClient(proxy *proxy.Proxy) (*http.Client, error) {
	// get FakeListener from proxy
	fakeListener, ok := proxy.GetListener().(*proxyFakes.FakeListener)
	if ok == false {
		return nil, errors.New("listener is not a fake listener")
	}

	transport := &http.Transport{}
	transport.DialContext = fakeListener.DialContext

	return &http.Client{
		Transport:     transport,
		CheckRedirect: nil,
		Jar:           nil,
		Timeout:       60 * time.Second,
	}, nil
}
