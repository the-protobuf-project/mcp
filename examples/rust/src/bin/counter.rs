//! CounterService MCP server over stdio.
//!
//! CounterService's only RPC is server-streaming, which is how the generated
//! code reports MCP progress. The crate's unit tests cover the handler in
//! isolation; this binary is what lets a real MCP client drive the progress
//! path end to end (see examples/conformance).
//!
//! Usage:
//!     cargo run --bin counter

use todo_mcp_example::counter_impl::CounterServer;
use todo_mcp_example::counter_service_mcp;

#[tokio::main]
async fn main() -> Result<(), Box<dyn std::error::Error>> {
    counter_service_mcp::serve_counter_service_mcp_stdio(CounterServer).await
}
