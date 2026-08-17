# Asset Gallery

This gallery serves the same kind of thing — an asset — under six different
IANA media types, so an MCP client has something concrete to render for each.

| Asset | Media type | Rendered as |
| --- | --- | --- |
| `overview` | `text/markdown` | prose |
| `report` | `text/html` | a document |
| `manifest` | `application/json` | structured data |
| `downloads` | `text/csv` | a table |
| `logo` | `image/png` | an inline image |
| `spec` | `application/pdf` | a download |

The media type is a plain string, not an enum: the IANA registry grows
without this schema changing.
