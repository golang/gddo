package runtimeconfig

import (
	"context"
	"fmt"
	"net"
	"strings"
	"testing"
	"time"

	tspb "github.com/golang/protobuf/ptypes/timestamp"
	"github.com/google/go-cmp/cmp"
	pb "google.golang.org/genproto/googleapis/cloud/runtimeconfig/v1beta1"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// Set wait timeout used for tests.
var watchOpt = &WatchOptions{
	WaitTime: 500 * time.Millisecond,
}

// fakeServer partially implements RuntimeConfigManagerServer for Client to connect to.  Prefill
// responses field with the ordered list of responses to GetVariable calls.
type fakeServer struct {
	pb.RuntimeConfigManagerServer
	responses []response
	index     int
}

type response struct {
	vrbl *pb.Variable
	err  error
}

func (s *fakeServer) GetVariable(context.Context, *pb.GetVariableRequest) (*pb.Variable, error) {
	if len(s.responses) == 0 {
		return nil, fmt.Errorf("fakeClient missing responses")
	}
	resp := s.responses[s.index]
	// Adjust index to next response for next call till it gets to last one, then keep using the
	// last one.
	if s.index < len(s.responses)-1 {
		s.index++
	}
	return resp.vrbl, resp.err
}

func setUp(t *testing.T, fs *fakeServer) (*Client, func()) {
	// TODO: Replace logic to use a port picker.
	const address = "localhost:8888"
	// Set up gRPC server.
	lis, err := net.Listen("tcp", address)
	if err != nil {
		t.Fatalf("tcp listen on %s failed: %v", address, err)
	}
	s := grpc.NewServer()
	pb.RegisterRuntimeConfigManagerServer(s, fs)
	// Run gRPC server on a background goroutine.
	go s.Serve(lis)

	// Set up client.
	conn, err := grpc.Dial(address, grpc.WithInsecure())
	if err != nil {
		t.Fatalf("did not connect: %v", err)
	}
	return &Client{
			conn:   conn,
			client: pb.NewRuntimeConfigManagerClient(conn),
		}, func() {
			conn.Close()
			s.Stop()
			time.Sleep(time.Second * 1)
		}
}

func pbToVariable(vpb *pb.Variable) (*Variable, error) {
	vrbl := &Variable{}
	tm, err := parseUpdateTime(vpb)
	if err != nil {
		return nil, err
	}
	copyFromProto(vpb, vrbl, tm)
	return vrbl, nil
}

var (
	startTime = time.Now().Unix()
	vrbl1     = &pb.Variable{
		Name:       "greetings",
		Contents:   &pb.Variable_Text{"hello"},
		UpdateTime: &tspb.Timestamp{Seconds: startTime},
	}
	vrbl2 = &pb.Variable{
		Name:       "greetings",
		Contents:   &pb.Variable_Text{"world"},
		UpdateTime: &tspb.Timestamp{Seconds: startTime + 100},
	}
)

func TestNewWatcher(t *testing.T) {
	client, cleanUp := setUp(t, &fakeServer{
		responses: []response{
			{vrbl: vrbl1},
		},
	})
	defer cleanUp()

	ctx := context.Background()
	w, err := client.NewWatcher(ctx, "projectID", "config", "greetings", watchOpt)
	if err != nil {
		t.Fatalf("Client.NewWatcher() returned error: %v", err)
	}

	got := w.Variable()
	want, err := pbToVariable(vrbl1)
	if err != nil {
		t.Fatal(err)
	}
	if diff := cmp.Diff(&got, want); diff != "" {
		t.Errorf("Watcher.Variable(): %s", diff)
	}
}

func TestWatchUpdatesVariable(t *testing.T) {
	client, cleanUp := setUp(t, &fakeServer{
		responses: []response{
			{vrbl: vrbl1},
			{vrbl: vrbl2},
		},
	})
	defer cleanUp()

	ctx := context.Background()
	w, err := client.NewWatcher(ctx, "projectID", "config", "greetings", watchOpt)
	if err != nil {
		t.Fatalf("Client.NewWatcher() returned error: %v", err)
	}

	if err := w.Watch(ctx); err != nil {
		t.Fatalf("Watcher.Watch() returned error: %v", err)
	}
	got := w.Variable()
	want, err := pbToVariable(vrbl2)
	if err != nil {
		t.Fatal(err)
	}
	if diff := cmp.Diff(&got, want); diff != "" {
		t.Errorf("Watcher.Variable(): %s", diff)
	}
}

func TestWatchVariableDeletedAndReset(t *testing.T) {
	client, cleanUp := setUp(t, &fakeServer{
		responses: []response{
			{vrbl: vrbl1},
			{err: status.Error(codes.NotFound, "deleted")},
			{vrbl: vrbl2},
		},
	})
	defer cleanUp()

	ctx := context.Background()
	w, err := client.NewWatcher(ctx, "projectID", "config", "greetings", watchOpt)
	if err != nil {
		t.Fatalf("Client.NewWatcher() returned error: %v", err)
	}

	// Expect deleted error.
	if err := w.Watch(ctx); err == nil {
		t.Fatalf("Watcher.Watch() returned nil, want error")
	} else {
		if !IsDeleted(err) {
			t.Fatalf("Watcher.Watch() returned error %v, want errDeleted", err)
		}
	}

	// Variable Name and Value should be the same, IsDeleted and UpdateTime should be updated.
	got := w.Variable()
	prev, err := pbToVariable(vrbl1)
	if err != nil {
		t.Fatal(err)
	}
	if got.Name != prev.Name {
		t.Errorf("Variable.Name got %v, want %v", got.Name, prev.Name)
	}
	if diff := cmp.Diff(got.Value, prev.Value); diff != "" {
		t.Errorf("Variable.Value: %s", diff)
	}
	if !got.IsDeleted {
		t.Errorf("Variable.IsDeleted got %v, want true", got.IsDeleted)
	}
	if !got.UpdateTime.After(prev.UpdateTime) {
		t.Errorf("Variable.UpdateTime is less than or equal to previous value")
	}

	// Calling Watch again will produce vrbl2.
	if err := w.Watch(ctx); err != nil {
		t.Fatalf("Watcher.Watch() returned error: %v", err)
	}
	got = w.Variable()
	want, err := pbToVariable(vrbl2)
	if err != nil {
		t.Fatal(err)
	}
	if diff := cmp.Diff(&got, want); diff != "" {
		t.Errorf("Watcher.Variable(): %s", diff)
	}
}

func TestWatchCancelled(t *testing.T) {
	client, cleanUp := setUp(t, &fakeServer{
		responses: []response{
			{vrbl: vrbl1},
		},
	})
	defer cleanUp()

	ctx := context.Background()
	w, err := client.NewWatcher(ctx, "projectID", "config", "greetings", watchOpt)
	if err != nil {
		t.Fatalf("Client.NewWatcher() returned error: %v", err)
	}

	ctx, cancel := context.WithCancel(ctx)
	go func() {
		time.Sleep(1 * time.Second)
		cancel()
	}()

	if err := w.Watch(ctx); err != context.Canceled {
		t.Fatalf("Watcher.Watch() returned %v, want context.Canceled", err)
	}
}

func TestWatchRPCError(t *testing.T) {
	rpcErr := status.Error(codes.Internal, "bad")
	client, cleanUp := setUp(t, &fakeServer{
		responses: []response{
			{vrbl: vrbl1},
			{err: rpcErr},
		},
	})
	defer cleanUp()

	ctx := context.Background()
	w, err := client.NewWatcher(ctx, "projectID", "config", "greetings", watchOpt)
	if err != nil {
		t.Fatalf("Client.NewWatcher() returned error: %v", err)
	}

	// Expect RPC error.
	err = w.Watch(ctx)
	if !strings.Contains(err.Error(), "bad") {
		t.Errorf("Watcher.Watch() returned %v, want %v", err, rpcErr)
	}

	// Variable should still be the same.
	got := w.Variable()
	want, err := pbToVariable(vrbl1)
	if err != nil {
		t.Fatal(err)
	}
	if diff := cmp.Diff(&got, want); diff != "" {
		t.Errorf("Watcher.Variable(): %s", diff)
	}
}
