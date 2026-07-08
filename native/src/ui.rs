//! Embedded web UI: serves the vite build from ../web/dist with an SPA
//! fallback to index.html, mirroring registerUI in internal/app/web.go.

use axum::http::{header, StatusCode, Uri};
use axum::response::{IntoResponse, Response};
use rust_embed::RustEmbed;

#[derive(RustEmbed)]
#[folder = "$CARGO_MANIFEST_DIR/../web/dist"]
struct WebAssets;

pub async fn static_handler(uri: Uri) -> Response {
    let path = uri.path().trim_start_matches('/');
    if !path.is_empty() {
        if let Some(asset) = WebAssets::get(path) {
            let mime = mime_guess::from_path(path).first_or_octet_stream();
            return (
                [(header::CONTENT_TYPE, mime.as_ref().to_string())],
                asset.data.into_owned(),
            )
                .into_response();
        }
    }
    // SPA shell for every unknown path.
    match WebAssets::get("index.html") {
        Some(index) => (
            [
                (header::CONTENT_TYPE, "text/html; charset=utf-8".to_string()),
                (
                    header::CACHE_CONTROL,
                    "no-store, must-revalidate".to_string(),
                ),
            ],
            index.data.into_owned(),
        )
            .into_response(),
        None => (
            StatusCode::SERVICE_UNAVAILABLE,
            "web UI not built — run `npm run build` in web/",
        )
            .into_response(),
    }
}
