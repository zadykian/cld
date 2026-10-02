package update

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"path"
	"strconv"
	"strings"
	"time"

	"github.com/zadykian/cld/internal/fail"
)

// release is a release's version, X.Y.Z.
type release [3]int

// parse reads a release's version, X.Y.Z, each a decimal number.
func parse(version string) (release, bool) {
	var r release
	parts := strings.Split(version, ".")
	if len(parts) != len(r) {
		return r, false
	}
	for i, part := range parts {
		if part == "" || strings.Trim(part, "0123456789") != "" {
			return r, false
		}
		n, err := strconv.Atoi(part)
		if err != nil {
			return r, false
		}
		r[i] = n
	}
	return r, true
}

func (r release) before(other release) bool {
	for i := range r {
		if r[i] != other[i] {
			return r[i] < other[i]
		}
	}
	return false
}

func (r release) String() string {
	return fmt.Sprintf("%d.%d.%d", r[0], r[1], r[2])
}

// latest is the latest release: the tag that the releases' latest redirects to.
func (u *updater) latest(ctx context.Context) (release, error) {
	address := u.releases + "/latest"
	notFound := func(why string) error {
		return fail.Runtime("cannot find the latest release at " + address + ": " + why)
	}
	client := &http.Client{
		Transport:     transport,
		CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, address, nil)
	if err != nil {
		return release{}, notFound(reason(err))
	}
	response, err := client.Do(request)
	if err != nil {
		return release{}, notFound(reason(err))
	}
	_ = response.Body.Close() //nolint:errcheck // a response's close loses nothing
	location, err := response.Location()
	if err != nil {
		return release{}, notFound(response.Status)
	}
	tag := path.Base(location.Path)
	latest, ok := parse(strings.TrimPrefix(tag, "v"))
	if !ok || !strings.HasPrefix(tag, "v") {
		return release{}, fail.Runtime(fmt.Sprintf("%s leads to '%s', not to a release", address, tag))
	}
	return latest, nil
}

// transport is Go's default, which takes a proxy from the environment, but gives up on a server
// that has not begun to answer within 30 seconds (decision 21.5).
var transport = func() *http.Transport {
	t := http.DefaultTransport.(*http.Transport).Clone()
	t.ResponseHeaderTimeout = 30 * time.Second
	return t
}()

// download writes the file at address to w, following redirects to HTTPS addresses only
// (decision 21.5).
func download(ctx context.Context, address string, w io.Writer) error {
	client := &http.Client{
		Transport: transport,
		CheckRedirect: func(request *http.Request, via []*http.Request) error {
			if request.URL.Scheme != "https" {
				return errors.New("redirected to " + request.URL.String() + ", which is not HTTPS")
			}
			if len(via) >= 10 {
				return errors.New("stopped after 10 redirects")
			}
			return nil
		},
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, address, nil)
	if err != nil {
		return fail.Runtime("cannot download " + address + ": " + reason(err))
	}
	response, err := client.Do(request)
	if err != nil {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		return fail.Runtime("cannot download " + address + ": " + reason(err))
	}
	defer response.Body.Close() //nolint:errcheck // a response's close loses nothing
	if response.StatusCode != http.StatusOK {
		return fail.Runtime("cannot download " + address + ": " + response.Status)
	}
	if _, err := io.Copy(w, response.Body); err != nil {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		return fail.Runtime("cannot download " + address + ": " + reason(err))
	}
	return nil
}
