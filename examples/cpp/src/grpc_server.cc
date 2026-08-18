#include "grpc_server.h"

#include <grpcpp/grpcpp.h>
#include <grpcpp/ext/proto_server_reflection_plugin.h>
#include <google/protobuf/timestamp.pb.h>
#include <iostream>
#include <memory>
#include <string>
#include <thread>

namespace todo {
namespace v1 {

TodoServiceGrpcImpl::TodoServiceGrpcImpl() {}

void TodoServiceGrpcImpl::fill_todo(Todo* out, const TodoItem& item) {
    out->set_name(item.name);
    out->set_title(item.title);
    out->set_description(item.description);
    out->set_completed(item.completed);

    if (item.priority == "PRIORITY_LOW") out->set_priority(PRIORITY_LOW);
    else if (item.priority == "PRIORITY_MEDIUM") out->set_priority(PRIORITY_MEDIUM);
    else if (item.priority == "PRIORITY_HIGH") out->set_priority(PRIORITY_HIGH);
    else out->set_priority(PRIORITY_UNSPECIFIED);

    auto* ct = out->mutable_create_time();
    ct->set_seconds(item.create_time);
    auto* ut = out->mutable_update_time();
    ut->set_seconds(item.update_time);
}

::grpc::Status TodoServiceGrpcImpl::CreateTodo(
    ::grpc::ServerContext* /*ctx*/, const CreateTodoRequest* req, Todo* resp) {
    auto todo = req->has_todo() ? req->todo() : Todo();
    std::string pri;
    switch (todo.priority()) {
        case PRIORITY_LOW:    pri = "PRIORITY_LOW"; break;
        case PRIORITY_MEDIUM: pri = "PRIORITY_MEDIUM"; break;
        case PRIORITY_HIGH:   pri = "PRIORITY_HIGH"; break;
        default:              pri = "PRIORITY_UNSPECIFIED"; break;
    }
    auto item = store_.create(
        req->parent(), req->todo_id(),
        todo.title(), todo.description(), todo.completed(), pri);
    fill_todo(resp, item);
    return ::grpc::Status::OK;
}

::grpc::Status TodoServiceGrpcImpl::GetTodo(
    ::grpc::ServerContext* /*ctx*/, const GetTodoRequest* req, Todo* resp) {
    auto* item = store_.get(req->name());
    if (!item) return {::grpc::StatusCode::NOT_FOUND, "todo not found: " + req->name()};
    fill_todo(resp, *item);
    return ::grpc::Status::OK;
}

::grpc::Status TodoServiceGrpcImpl::ListTodos(
    ::grpc::ServerContext* /*ctx*/, const ListTodosRequest* /*req*/,
    ListTodosResponse* resp) {
    for (const auto& item : store_.list()) {
        fill_todo(resp->add_todos(), item);
    }
    return ::grpc::Status::OK;
}

::grpc::Status TodoServiceGrpcImpl::UpdateTodo(
    ::grpc::ServerContext* /*ctx*/, const UpdateTodoRequest* req, Todo* resp) {
    if (!req->has_todo())
        return {::grpc::StatusCode::INVALID_ARGUMENT, "missing todo"};
    const auto& upd = req->todo();
    auto* existing = store_.update(upd.name());
    if (!existing) return {::grpc::StatusCode::NOT_FOUND, "todo not found: " + upd.name()};

    if (!upd.title().empty()) existing->title = upd.title();
    if (!upd.description().empty()) existing->description = upd.description();
    existing->completed = upd.completed();
    if (upd.priority() != PRIORITY_UNSPECIFIED) {
        switch (upd.priority()) {
            case PRIORITY_LOW:    existing->priority = "PRIORITY_LOW"; break;
            case PRIORITY_MEDIUM: existing->priority = "PRIORITY_MEDIUM"; break;
            case PRIORITY_HIGH:   existing->priority = "PRIORITY_HIGH"; break;
            default: break;
        }
    }
    fill_todo(resp, *existing);
    return ::grpc::Status::OK;
}

::grpc::Status TodoServiceGrpcImpl::DeleteTodo(
    ::grpc::ServerContext* /*ctx*/, const DeleteTodoRequest* req,
    ::google::protobuf::Empty* /*resp*/) {
    if (!store_.remove(req->name()))
        return {::grpc::StatusCode::NOT_FOUND, "todo not found: " + req->name()};
    return ::grpc::Status::OK;
}

void start_grpc_server(const std::string& addr) {
    auto service = std::make_shared<TodoServiceGrpcImpl>();

    std::thread([addr, service]() {
        grpc::reflection::InitProtoReflectionServerBuilderPlugin();
        ::grpc::ServerBuilder builder;
        builder.AddListeningPort(addr, ::grpc::InsecureServerCredentials());
        builder.RegisterService(service.get());
        auto server = builder.BuildAndStart();
        std::cerr << "gRPC server listening on " << addr << std::endl;
        server->Wait();
    }).detach();
}

}  // namespace v1
}  // namespace todo
