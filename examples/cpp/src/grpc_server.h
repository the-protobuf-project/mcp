#pragma once

#include <string>
#include "todo_store.h"
#include "todo/v1/todo_service.grpc.pb.h"

namespace todo {
namespace v1 {

class TodoServiceGrpcImpl final : public TodoService::Service {
public:
    TodoServiceGrpcImpl();

    ::grpc::Status CreateTodo(::grpc::ServerContext* ctx,
                              const CreateTodoRequest* req,
                              Todo* resp) override;

    ::grpc::Status GetTodo(::grpc::ServerContext* ctx,
                           const GetTodoRequest* req,
                           Todo* resp) override;

    ::grpc::Status ListTodos(::grpc::ServerContext* ctx,
                             const ListTodosRequest* req,
                             ListTodosResponse* resp) override;

    ::grpc::Status UpdateTodo(::grpc::ServerContext* ctx,
                              const UpdateTodoRequest* req,
                              Todo* resp) override;

    ::grpc::Status DeleteTodo(::grpc::ServerContext* ctx,
                              const DeleteTodoRequest* req,
                              ::google::protobuf::Empty* resp) override;

private:
    TodoStore store_;
    static void fill_todo(Todo* out, const TodoItem& item);
};

// Starts the gRPC server on a background thread. Pure C++, no FFI.
void start_grpc_server(const std::string& addr);

}  // namespace v1
}  // namespace todo
