package main

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/the-protobuf-project/mcp/examples/proto/generated/go/todo/todopbv1"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/emptypb"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// Compile-time checks: todoServer implements both the MCP and gRPC server interfaces.
var _ todopbv1.TodoServiceMCPServer = (*todoServer)(nil)
var _ todopbv1.TodoServiceServer = (*todoServer)(nil)

// todoServer is an in-memory TodoService implementation.
type todoServer struct {
	todopbv1.UnimplementedTodoServiceServer
	mu    sync.RWMutex
	todos map[string]*todopbv1.Todo
}

func newTodoServer() *todoServer {
	return &todoServer{todos: make(map[string]*todopbv1.Todo)}
}

func (s *todoServer) CreateTodo(_ context.Context, req *todopbv1.CreateTodoRequest) (*todopbv1.Todo, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	name := fmt.Sprintf("%s/todos/%s", req.GetParent(), req.GetTodoId())
	now := timestamppb.New(time.Now())

	todo := req.GetTodo()
	if todo == nil {
		todo = &todopbv1.Todo{}
	}
	todo.Name = name
	todo.CreateTime = now
	todo.UpdateTime = now

	s.todos[name] = todo
	return todo, nil
}

func (s *todoServer) GetTodo(_ context.Context, req *todopbv1.GetTodoRequest) (*todopbv1.Todo, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	todo, ok := s.todos[req.GetName()]
	if !ok {
		return nil, fmt.Errorf("todo %q not found", req.GetName())
	}
	return todo, nil
}

func (s *todoServer) ListTodos(_ context.Context, req *todopbv1.ListTodosRequest) (*todopbv1.ListTodosResponse, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	prefix := req.GetParent() + "/todos/"
	var result []*todopbv1.Todo
	for _, t := range s.todos {
		if strings.HasPrefix(t.GetName(), prefix) {
			result = append(result, t)
		}
	}
	return &todopbv1.ListTodosResponse{Todos: result}, nil
}

func (s *todoServer) UpdateTodo(_ context.Context, req *todopbv1.UpdateTodoRequest) (*todopbv1.Todo, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	existing, ok := s.todos[req.GetTodo().GetName()]
	if !ok {
		return nil, fmt.Errorf("todo %q not found", req.GetTodo().GetName())
	}

	t := req.GetTodo()
	mask := req.GetUpdateMask().GetPaths()
	if len(mask) == 0 {
		// No mask: full replace (preserve name and timestamps).
		name := existing.Name
		createTime := existing.CreateTime
		proto.Reset(existing)
		proto.Merge(existing, t)
		existing.Name = name
		existing.CreateTime = createTime
	} else {
		for _, path := range mask {
			switch path {
			case "title":
				existing.Title = t.Title
			case "description":
				existing.Description = t.Description
			case "completed":
				existing.Completed = t.Completed
			case "priority":
				existing.Priority = t.Priority
			}
		}
	}
	existing.UpdateTime = timestamppb.New(time.Now())

	return existing, nil
}

func (s *todoServer) DeleteTodo(_ context.Context, req *todopbv1.DeleteTodoRequest) (*emptypb.Empty, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if _, ok := s.todos[req.GetName()]; !ok {
		return nil, fmt.Errorf("todo %q not found", req.GetName())
	}
	delete(s.todos, req.GetName())
	return &emptypb.Empty{}, nil
}
