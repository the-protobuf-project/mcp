//! Type-checks the gallery's generated Rust MCP handler.
//!
//! The generated `resources()` builds each declared resource with
//! `serde_json::from_value` into rmcp's `Resource`, so compiling here proves the
//! media types, titles, sizes, annotations and icons in the proto produce
//! structures rmcp actually accepts.

pub mod proto;

#[path = "../../gen/rust/gallery/v1/gallery_service.mcp.rs"]
#[allow(dead_code)]
pub mod gallery_service_mcp;

#[cfg(test)]
mod tests {
    use super::gallery_service_mcp::GalleryServiceMcpHandler;

    // The generated resources() parses each declaration into rmcp's Resource at
    // run time and panics on a bad one, so compiling is not enough — this calls
    // it. It is also the Rust half of the media-type parity check against Go and
    // Python.
    #[test]
    fn resources_carry_every_declared_media_type() {
        let resources = GalleryServiceMcpHandler::<crate::tests::Stub>::resources();

        let mut types: Vec<&str> = resources
            .iter()
            .filter_map(|r| r.mime_type.as_deref())
            .collect();
        types.sort_unstable();

        assert_eq!(
            types,
            vec![
                "application/json",
                "application/pdf",
                "image/png",
                "text/csv",
                "text/html", // the gallery's HTML document
                "text/html", // and the MCP App UI resource
                "text/markdown",
            ],
            "declared media types must survive into the resource list"
        );
    }

    // Behavioural hints tell a client whether a call needs confirmation. A hint
    // the proto does not state must stay absent rather than default to false.
    #[test]
    fn tools_carry_behavioural_hints() {
        let tools = GalleryServiceMcpHandler::<crate::tests::Stub>::tools();
        let listed = tools
            .iter()
            .find(|t| t.name.as_ref() == "list_assets")
            .expect("list_assets missing from tools()");

        let ann = listed
            .annotations
            .as_ref()
            .expect("list_assets has no annotations");
        assert_eq!(ann.read_only_hint, Some(true));
        assert_eq!(ann.idempotent_hint, Some(true));
        assert_eq!(ann.open_world_hint, Some(false), "stated false must survive");
        assert_eq!(
            ann.destructive_hint, None,
            "unstated hint must stay absent, not default"
        );
    }

    // The URI-template resource is listed separately from concrete resources.
    #[test]
    fn resource_template_is_registered() {
        let templates = GalleryServiceMcpHandler::<crate::tests::Stub>::resource_templates();
        assert_eq!(templates.len(), 1);
        assert_eq!(templates[0].uri_template, "gallery://assets/{asset}");
        assert_eq!(
            templates[0].mime_type.as_deref(),
            Some("application/octet-stream")
        );
    }

    // Metadata declared in the proto must reach the client, not just the type.
    #[test]
    fn overview_carries_annotations_and_icons() {
        let resources = GalleryServiceMcpHandler::<crate::tests::Stub>::resources();
        let overview = resources
            .iter()
            .find(|r| r.uri == "gallery://docs/overview.md")
            .expect("overview resource missing");

        assert_eq!(overview.title.as_deref(), Some("Gallery overview"));
        assert_eq!(overview.size, Some(596));

        let annotations = overview
            .annotations
            .as_ref()
            .expect("annotations dropped between proto and resource");
        assert_eq!(annotations.priority, Some(1.0));
        assert_eq!(
            annotations.audience.as_ref().map(|a| a.len()),
            Some(2),
            "audience should hold both user and assistant"
        );

        let icons = overview.icons.as_ref().expect("icons dropped");
        assert_eq!(icons.len(), 2, "a light and a dark variant");
    }

    // A stub is needed only to name the handler's type parameter; no method on
    // it is ever called by the resource constructors under test.
    pub struct Stub;

    #[async_trait::async_trait]
    impl super::gallery_service_mcp::GalleryServiceMcpServer for Stub {
        async fn list_assets(
            &self,
            _args: serde_json::Value,
        ) -> std::result::Result<serde_json::Value, rmcp::ErrorData> {
            unreachable!("resource construction does not invoke tools")
        }
        async fn get_asset(
            &self,
            _args: serde_json::Value,
        ) -> std::result::Result<serde_json::Value, rmcp::ErrorData> {
            unreachable!("resource construction does not invoke tools")
        }
    }
}
