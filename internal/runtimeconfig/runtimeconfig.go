// Package runtimeconfig provides a limited set of client-side APIs for the Cloud Runtime
// Configurator. The Cloud Runtime Configurator service allows projects to store runtime
// configurations in the cloud and have clients fetch and get notified of changes to configuration
// values during runtime.
//
// This package provides a Client that sets up a Watcher for detecting updates on a Runtime
// Configurator variable.
package runtimeconfig

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/golang/protobuf/ptypes"
	"google.golang.org/api/option"
	transport "google.golang.org/api/transport/grpc"
	pb "google.golang.org/genproto/googleapis/cloud/runtimeconfig/v1beta1"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

const (
	// endpoint is the address of the GCP Runtime Configurator API.
	endPoint = "runtimeconfig.googleapis.com:443"
	// defaultWaitTimeout is the default value for WatchOptions.WaitTime if not set.
	defaultWaitTimeout = 10 * time.Minute
)

// List of authentication scopes required for using the Runtime Configurator API.
var authScopes = []string{
	"https://www.googleapis.com/auth/cloud-platform",
	"https://www.googleapis.com/auth/cloudruntimeconfig",
}

// Client is a RuntimeConfigManager client.  It wraps the gRPC client stub and currently exposes
// only a few APIs primarily for fetching and watching over configuration variables.
type Client struct {
	conn *grpc.ClientConn
	// The gRPC API client.
	client pb.RuntimeConfigManagerClient
}

// NewClient constructs a Client instance from given gRPC connection.
func NewClient(ctx context.Context, opts ...option.ClientOption) (*Client, error) {
	opts = append(opts, option.WithEndpoint(endPoint), option.WithScopes(authScopes...))
	conn, err := transport.Dial(ctx, opts...)
	if err != nil {
		return nil, err
	}
	return &Client{
		conn:   conn,
		client: pb.NewRuntimeConfigManagerClient(conn),
	}, nil
}

// Close tears down the gRPC connection used by this Client.
func (c *Client) Close() error {
	return c.conn.Close()
}

// NewWatcher will fetch variable for given projectID, configName and varName, then constructs a
// Watcher object containing the fetched variable.  Users can then use the Watcher to retrieve the
// variable as well as wait for changes.
func (c *Client) NewWatcher(ctx context.Context, projectID, configName, varName string,
	opts *WatchOptions) (*Watcher, error) {

	name := fmt.Sprintf("projects/%s/configs/%s/variables/%s", projectID, configName, varName)
	vpb, err := c.client.GetVariable(ctx, &pb.GetVariableRequest{Name: name})
	if err != nil {
		return nil, err
	}

	if opts == nil {
		opts = &WatchOptions{}
	}
	waitTime := opts.WaitTime
	switch {
	case waitTime == 0:
		waitTime = defaultWaitTimeout
	case waitTime < 0:
		return nil, fmt.Errorf("cannot have negative WaitTime option value: %v", waitTime)
	}

	// Make sure update time is valid before copying.
	updateTime, err := parseUpdateTime(vpb)
	if err != nil {
		return nil, err
	}

	w := &Watcher{
		client:      c.client,
		waitTime:    waitTime,
		lastRPCTime: time.Now(),
	}
	copyFromProto(vpb, &w.vrbl, updateTime)
	return w, nil
}

// WatchOptions provide optional configurations to the Watcher.
type WatchOptions struct {
	// WaitTime controls the frequency of making RPC and checking for updates by the Watch method.
	// A Watcher keeps track of the last time it made an RPC, when Watch is called, it waits for
	// configured WaitTime from the last RPC before making another RPC. The smaller the value, the
	// higher the frequency of making RPCs, which also means faster rate of hitting the API quota.
	//
	// If this option is not set or set to 0, it uses defaultWaitTimeout value.
	WaitTime time.Duration
}

// Watcher caches a variable in memory and listens for updates from the Runtime Configurator
// service.
type Watcher struct {
	client      pb.RuntimeConfigManagerClient
	waitTime    time.Duration
	lastRPCTime time.Time

	mu   sync.Mutex
	vrbl Variable
}

// Variable returns a shallow copy of the associated variable of this Watcher object.  It is safe to
// use from multiple goroutines.
func (w *Watcher) Variable() Variable {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.vrbl
}

var errDeleted = errors.New("deleted variable")

// IsDeleted returns true if variable has been deleted.
func IsDeleted(err error) bool {
	return err == errDeleted
}

// Watch blocks until the variable changes, the Context's Done channel closes, or an RPC
// error occurs.
//
// If the variable has a new value, then method returns nil and the value can be retrieved by
// calling Variable.
//
// If the variable is deleted, then method returns an error that will be matched by IsDeleted.
// Subsequent calls to this method will block until the variable is restored or another error
// occurs.
//
// It is NOT safe to call this method from multiple goroutines.
//
// To stop this function from blocking, caller can passed in Context object constructed via
// WithCancel and call the cancel function.
func (w *Watcher) Watch(ctx context.Context) error {
	// Loop to check for changes or continue waiting.
	for {
		// Block until waitTime or context cancelled/timed out.
		waitTime := w.waitTime - time.Now().Sub(w.lastRPCTime)
		select {
		case <-time.After(waitTime):
		case <-ctx.Done():
			return ctx.Err()
		}

		// Use GetVariables RPC and check for deltas based on the response. May consider using
		// ListVariables RPC with Filter=<key> and ReturnValues=false to identify deltas before
		// doing a GetVariable to potentially save on response size. However, even with
		// Filter=<key>, the response on ListVariables may return more than one matching variable
		// and this code will need to iterate through calling more ListVariables RPCs.
		vpb, err := w.client.GetVariable(ctx, &pb.GetVariableRequest{Name: w.vrbl.Name})
		w.lastRPCTime = time.Now()
		if err == nil {
			updateTime, err := parseUpdateTime(vpb)
			if err != nil {
				return err
			}

			// Determine if there are any changes based on update_time field. If there are, update
			// cache and return nil, else continue on.
			// TODO(herbie): It is currently possible to have update_time changed but without any
			// changes in the content. Need to re-evaluate if this should instead check for actual
			// content changes.
			w.mu.Lock()
			if !w.vrbl.UpdateTime.Equal(updateTime) {
				copyFromProto(vpb, &w.vrbl, updateTime)
				w.mu.Unlock()
				return nil
			}
			w.mu.Unlock()

		} else {
			if st, ok := status.FromError(err); !ok || st.Code() != codes.NotFound {
				return err
			}
			// For RPC not found error, if last known state is not deleted, update and return
			// errDeleted, else treat as no change has occurred.
			w.mu.Lock()
			if !w.vrbl.IsDeleted {
				w.vrbl.IsDeleted = true
				w.vrbl.UpdateTime = time.Now().UTC()
				w.mu.Unlock()
				return errDeleted
			}
			w.mu.Unlock()
		}
	}
}

// Variable contains the runtime configuration data.
// Treat Value field as read-only.  Writes to it may affect other Variable objects containing
// reference to the same backing array.
type Variable struct {
	Name       string
	Value      []byte
	IsDeleted  bool
	UpdateTime time.Time
}

func copyFromProto(vpb *pb.Variable, vrbl *Variable, updateTime time.Time) {
	vrbl.Name = vpb.Name
	vrbl.UpdateTime = updateTime
	vrbl.IsDeleted = false
	vrbl.Value = vpb.GetValue()
	// We currently only expose content in []byte. If proto contains text content, convert that to
	// []byte.
	if _, isText := vpb.GetContents().(*pb.Variable_Text); isText {
		vrbl.Value = []byte(vpb.GetText())
	}
}

func parseUpdateTime(vpb *pb.Variable) (time.Time, error) {
	updateTime, err := ptypes.Timestamp(vpb.GetUpdateTime())
	if err != nil {
		return time.Time{}, fmt.Errorf(
			"variable message for name=%q contains invalid timestamp: %v", vpb.Name, err)
	}
	return updateTime, nil
}
