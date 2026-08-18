//! A CounterService implementation exercising the generated streaming path.
//!
//! The generated trait hands a [`McpProgressSink`] to a server-streaming RPC;
//! the implementation reports each step through it and returns the final
//! result. When the MCP client sent no progressToken the sink is inert, so
//! there is nothing to branch on here.

use async_trait::async_trait;
use rmcp::ErrorData as McpError;
use serde_json::{json, Value};

use crate::counter_service_mcp::{CounterServiceMcpServer, McpProgressSink};

pub struct CounterServer;

#[async_trait]
impl CounterServiceMcpServer for CounterServer {
    async fn count(
        &self,
        args: Value,
        progress: McpProgressSink,
    ) -> std::result::Result<Value, McpError> {
        let to = args.get("to").and_then(Value::as_i64).unwrap_or(0);
        if to < 0 {
            return Err(McpError::invalid_params("`to` must not be negative", None));
        }

        for n in 1..=to {
            progress
                .send(n as f64, Some(to as f64), Some(format!("counted {n}")))
                .await;
        }

        Ok(json!({ "total": to }))
    }
}

#[cfg(test)]
mod tests {
    use super::*;
    use crate::counter_service_mcp::CounterServiceMcpHandler;

    // The bug this guards: server-streaming RPCs used to be filtered out of the
    // generated trait, tool list, and call_tool arm, so a progress service
    // produced a server with no tools at all. Compiling would not have caught
    // it — the code was simply absent.
    #[test]
    fn streaming_rpc_is_advertised_as_a_tool() {
        let tools = CounterServiceMcpHandler::<CounterServer>::tools();
        let names: Vec<&str> = tools.iter().map(|t| t.name.as_ref()).collect();
        assert!(
            names.contains(&"counter_service-count_v1"),
            "streaming RPC missing from tools(): {names:?}"
        );
    }

    // An inert sink stands in for a client that sent no progressToken; the RPC
    // must still run to completion and return its result.
    #[tokio::test]
    async fn count_returns_its_total_without_a_progress_token() {
        let sink = McpProgressSink::default();
        let out = CounterServer.count(json!({"to": 3}), sink).await.unwrap();
        assert_eq!(out, json!({"total": 3}));
    }

    #[tokio::test]
    async fn count_rejects_a_negative_target() {
        let sink = McpProgressSink::default();
        assert!(CounterServer.count(json!({"to": -1}), sink).await.is_err());
    }
}
