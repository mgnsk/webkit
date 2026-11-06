package app_test

import (
	"context"
	"errors"
	"net"
	"net/http"
	"sync/atomic"
	"testing"
	"time"

	"github.com/mgnsk/webkit/app"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func TestSuite(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "app")
}

func freeAddr() string {
	GinkgoHelper()
	l, err := net.Listen("tcp", "127.0.0.1:0")
	Expect(err).NotTo(HaveOccurred())
	defer l.Close()
	return l.Addr().String()
}

var _ = Describe("Run", func() {
	It("serves until the context is done", func(ctx SpecContext) {
		addr := freeAddr()
		srv := &http.Server{Addr: addr, Handler: http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusTeapot)
		})}

		runCtx, cancel := context.WithCancel(ctx)
		done := make(chan error, 1)
		go func() { done <- app.Run(runCtx, srv) }()

		Eventually(func() (int, error) {
			resp, err := http.Get("http://" + addr)
			if err != nil {
				return 0, err
			}
			defer resp.Body.Close()
			return resp.StatusCode, nil
		}).Should(Equal(http.StatusTeapot))

		cancel()
		Eventually(done).Should(Receive(BeNil()))
	}, SpecTimeout(5*time.Second))

	It("stops when a task fails", func(ctx SpecContext) {
		srv := &http.Server{Addr: freeAddr(), Handler: http.NotFoundHandler()}
		taskErr := errors.New("task failed")

		Expect(app.Run(ctx, srv, func(context.Context) error { return taskErr })).To(MatchError(taskErr))
	}, SpecTimeout(5*time.Second))
})

var _ = Describe("Every", func() {
	It("runs periodically until the context is done", func(ctx SpecContext) {
		var n atomic.Int32
		runCtx, cancel := context.WithCancel(ctx)
		done := make(chan error, 1)

		go func() {
			done <- app.Every(time.Millisecond, func(context.Context, time.Time) error {
				n.Add(1)
				return nil
			})(runCtx)
		}()

		Eventually(n.Load).Should(BeNumerically(">=", 3))
		cancel()
		Eventually(done).Should(Receive(BeNil()))
	}, SpecTimeout(5*time.Second))

	It("returns the first error", func(ctx SpecContext) {
		errTick := errors.New("tick")
		Expect(app.Every(time.Millisecond, func(context.Context, time.Time) error {
			return errTick
		})(ctx)).To(MatchError(errTick))
	}, SpecTimeout(5*time.Second))
})
