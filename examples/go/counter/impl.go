package main

import (
	"fmt"
	"time"

	mcppb "buf.build/gen/go/the-protobuf-project/mcp/protocolbuffers/go/mcp/protobuf"
	"github.com/the-protobuf-project/mcp/examples/proto/generated/go/counter/counterpbv1"
	"github.com/the-protobuf-project/runtime-go/agents/mcp"
	"google.golang.org/grpc"
	"google.golang.org/grpc/metadata"
)

// Compile-time check: counterServer implements CounterServiceServer.
var _ counterpbv1.CounterServiceServer = (*counterServer)(nil)

type counterServer struct {
	counterpbv1.UnimplementedCounterServiceServer
}

func newCounterServer() *counterServer {
	return &counterServer{}
}

// Count streams progress updates as it counts from 0 to req.To, then sends the final result.
// Only sends MCPProgress chunks when the client passed progressToken in gRPC metadata
// (mcp-progress-token), i.e. when the MCP client included it in params._meta.
func (s *counterServer) Count(req *counterpbv1.CountRequest, stream grpc.ServerStreamingServer[counterpbv1.CountStreamChunk]) error {
	to := req.GetTo()
	if to < 0 {
		to = 0
	}
	total := float64(to + 1)

	// Only send progress when client requested it via metadata (from params._meta.progressToken).
	md, _ := metadata.FromIncomingContext(stream.Context())
	wantsProgress := len(md.Get(mcp.GRPCProgressTokenKey)) > 0

	for i := int32(0); i <= to; i++ {
		select {
		case <-stream.Context().Done():
			return stream.Context().Err()
		default:
		}
		if wantsProgress {
			// Send progress update
			progress := float64(i + 1)
			chunk := &counterpbv1.CountStreamChunk{
				Payload: &counterpbv1.CountStreamChunk_Progress{
					Progress: &mcppb.MCPProgress{
						Progress: progress,
						Total:    &total,
						Message:  fmt.Sprintf("Counting... %d/%d", i, to),
					},
				},
			}
			if err := stream.Send(chunk); err != nil {
				return err
			}
		}
		time.Sleep(200 * time.Millisecond) // Simulate work
	}

	// Send final result
	chunk := &counterpbv1.CountStreamChunk{
		Payload: &counterpbv1.CountStreamChunk_Result{
			Result: &counterpbv1.CountResponse{Count: to},
		},
	}
	return stream.Send(chunk)
}
