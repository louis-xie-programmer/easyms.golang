package proxy

import (
	"net/http"
	"net/http/httputil"
	"net/url"
)

func NewReverseProxy(target string) (*httputil.ReverseProxy, error) {
	url, err := url.Parse(target)
	if err != nil {
		return nil, err
	}

	proxy := httputil.NewSingleHostReverseProxy(url)

	// 修改 Director 修正 Host 和 URL
	originalDirector := proxy.Director
	proxy.Director = func(req *http.Request) {
		originalDirector(req)

		// 设置正确的 Host，避免回流到网关
		req.Host = url.Host
		req.URL.Host = url.Host
		req.URL.Scheme = url.Scheme
	}

	return proxy, nil
}

func Forward(w http.ResponseWriter, r *http.Request, target string) {
	proxy, err := NewReverseProxy(target)
	if err != nil {
		http.Error(w, "invalid target", http.StatusBadGateway)
		return
	}

	proxy.ServeHTTP(w, r)
}
