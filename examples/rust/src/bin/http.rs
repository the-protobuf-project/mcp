//! TodoService MCP server over streamable-http + gRPC side-by-side.
//!
//! Usage:
//!     cargo run --bin http

use std::net::SocketAddr;

use todo_mcp_example::proto::todo_v1::todo_service_server::TodoServiceServer;
use todo_mcp_example::todo_impl::TodoServer;
use todo_mcp_example::todo_service_mcp;

#[tokio::main]
async fn main() -> Result<(), Box<dyn std::error::Error>> {
    let srv = TodoServer::new();

    let host = std::env::var("MCP_HOST").unwrap_or_else(|_| "0.0.0.0".into());
    let port: u16 = std::env::var("MCP_PORT")
        .ok()
        .and_then(|v| v.parse().ok())
        .unwrap_or(8082);

    // gRPC in background task (with reflection). The address is configurable so
    // that more than one example server can run at once -- a hard-coded port
    // makes the second one fail to bind.
    let grpc_addr: SocketAddr = std::env::var("GRPC_ADDR")
        .unwrap_or_else(|_| "[::]:50051".into())
        .parse()?;
    let grpc_srv = srv.clone();
    tokio::spawn(async move {
        const FILE_DESCRIPTOR_SET: &[u8] =
            include_bytes!("../../../proto/generated/rust/descriptor.binpb");
        let reflection_service = tonic_reflection::server::Builder::configure()
            .register_encoded_file_descriptor_set(FILE_DESCRIPTOR_SET)
            .build_v1()
            .expect("failed to build reflection service");
        eprintln!("gRPC listening on {grpc_addr} (reflection enabled)");
        tonic::transport::Server::builder()
            .add_service(TodoServiceServer::new(grpc_srv))
            .add_service(reflection_service)
            .serve(grpc_addr)
            .await
            .expect("gRPC server failed");
    });

    eprintln!("MCP starting (transport=streamable-http, {host}:{port})");
    todo_service_mcp::serve_todo_service_mcp(
        srv,
        todo_service_mcp::TodoServiceMcpTransportConfig {
            transport: "streamable-http".into(),
            host,
            port,
            // The proto-derived default, so this endpoint matches the Go and
            // C++ examples rather than drifting a path segment away from them.
            base_path: todo_service_mcp::TODO_SERVICE_MCP_DEFAULT_BASE_PATH.into(),
        },
    )
    .await
}
