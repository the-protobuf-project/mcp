pub mod proto;

// CounterService's stream chunk holds an mcp.MCPProgress. prost renders that as
// `super::super::super::mcp::McpProgress` — the crate root — so the generated
// mcp types are mounted here rather than under `proto`.
#[allow(clippy::all, non_camel_case_types)]
pub mod mcp {
    include!("../../proto/generated/rust/mcp/mcp.rs");
}

#[path = "../../proto/generated/rust/todo/v1/todo_service.mcp.rs"]
#[allow(dead_code)]
pub mod todo_service_mcp;

#[path = "impl.rs"]
pub mod todo_impl;

// CounterService is the streaming example: its one RPC reports MCP progress.
// Compiling it here is what keeps the generated streaming path honest — the
// todo service has no streaming RPC, so nothing else in this crate covers it.
#[path = "../../proto/generated/rust/counter/v1/counter_service.mcp.rs"]
#[allow(dead_code)]
pub mod counter_service_mcp;

#[path = "counter_impl.rs"]
pub mod counter_impl;
