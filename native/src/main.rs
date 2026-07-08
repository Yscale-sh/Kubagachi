//! kubagachi-native: bare-metal (macOS/Linux) server for the kubagachi cluster
//! cockpit. Serves the embedded web UI plus a live READ-ONLY cluster snapshot
//! from the local kubeconfig — a port of the Go server's read path.

mod critters;
mod k8s;
mod sprites;
mod ui;
mod wire;

use std::convert::Infallible;
use std::net::SocketAddr;
use std::path::PathBuf;
use std::sync::Arc;
use std::time::Duration;

use axum::extract::State;
use axum::http::{header, StatusCode};
use axum::response::sse::{Event, KeepAlive, Sse};
use axum::response::IntoResponse;
use axum::routing::{any, get};
use axum::{Json, Router};
use clap::Parser;
use futures::Stream;
use tokio::sync::watch;
use tokio_stream::wrappers::WatchStream;
use tokio_stream::StreamExt as _;
use tower_http::cors::CorsLayer;
use tower_http::services::ServeDir;

#[derive(Parser, Debug, Clone)]
#[command(
    name = "kubagachi-native",
    about = "Native kubagachi cockpit: embedded web UI + live read-only cluster snapshot"
)]
struct Args {
    /// Port to listen on.
    #[arg(long, default_value_t = 8080)]
    port: u16,
    /// Kubeconfig context name (defaults to the current context).
    #[arg(long)]
    context: Option<String>,
    /// Directory holding the critter sprite sheets.
    #[arg(long, default_value = "critters")]
    critters_dir: PathBuf,
    /// Namespace to watch (defaults to all namespaces).
    #[arg(long)]
    namespace: Option<String>,
    /// Open the browser to the served URL on start.
    #[arg(long, default_value_t = false)]
    open: bool,
}

struct AppState {
    snapshot_rx: watch::Receiver<Arc<String>>,
    critters_dir: PathBuf,
    kube_context: Option<String>,
    bounds_cache: sprites::BoundsCache,
}

#[tokio::main]
async fn main() -> anyhow::Result<()> {
    tracing_subscriber::fmt()
        .with_env_filter(
            tracing_subscriber::EnvFilter::try_from_default_env()
                .unwrap_or_else(|_| "info".into()),
        )
        .init();
    let args = Args::parse();

    // Critter name set: the manifest keys, sorted — the same pool the Go
    // server loads via LoadPixelSprites.
    let names = Arc::new(match critters::load_names(&args.critters_dir) {
        Ok(n) => n,
        Err(err) => {
            tracing::warn!("no critter manifest ({err}); pods will have no critter");
            Vec::new()
        }
    });

    // Kube client from the local kubeconfig, honoring --context.
    let kc_opts = kube::config::KubeConfigOptions {
        context: args.context.clone(),
        cluster: None,
        user: None,
    };
    let config = kube::Config::from_kubeconfig(&kc_opts).await?;
    let client = kube::Client::try_from(config)?;

    let context_name = args.context.clone().unwrap_or_else(|| {
        kube::config::Kubeconfig::read()
            .ok()
            .and_then(|kc| kc.current_context)
            .unwrap_or_default()
    });
    let version = match client.apiserver_version().await {
        Ok(info) => info.git_version,
        Err(err) => {
            tracing::warn!("apiserver version unavailable: {err}");
            String::new()
        }
    };

    let initial = wire::WebSnapshot::empty_live(
        &context_name,
        &version,
        args.namespace.as_deref().unwrap_or(""),
    );
    let (tx, rx) = watch::channel(Arc::new(serde_json::to_string(&initial)?));

    // Refresher: periodic LIST of pods + nodes, published to the watch channel.
    {
        let client = client.clone();
        let namespace = args.namespace.clone();
        let context_name = context_name.clone();
        let version = version.clone();
        let names = names.clone();
        tokio::spawn(async move {
            let mut tick = tokio::time::interval(Duration::from_secs(2));
            tick.set_missed_tick_behavior(tokio::time::MissedTickBehavior::Delay);
            loop {
                tick.tick().await;
                match k8s::build_snapshot(
                    &client,
                    namespace.as_deref(),
                    &context_name,
                    &version,
                    &names,
                )
                .await
                {
                    Ok(snap) => match serde_json::to_string(&snap) {
                        Ok(json) => {
                            let _ = tx.send(Arc::new(json));
                        }
                        Err(err) => tracing::warn!("snapshot encode failed: {err}"),
                    },
                    Err(err) => tracing::warn!("snapshot refresh failed: {err}"),
                }
            }
        });
    }

    let state = Arc::new(AppState {
        snapshot_rx: rx,
        critters_dir: args.critters_dir.clone(),
        kube_context: args.context.clone(),
        bounds_cache: sprites::BoundsCache::default(),
    });

    let router = Router::new()
        .route("/api/snapshot", get(api_snapshot))
        .route("/api/stream", get(api_stream))
        .route("/api/critters", get(api_critters))
        .route("/api/contexts", get(api_contexts))
        // Operate/write endpoints are out of scope in the native server: the
        // UI gets a clean 501 instead of a hang.
        .route("/api/contexts/select", any(not_implemented))
        .route("/api/object", any(not_implemented))
        .route("/api/object/apply", any(not_implemented))
        .route("/api/secret", any(not_implemented))
        .route("/api/customresources", any(not_implemented))
        .route("/api/resource/*rest", any(not_implemented))
        .route("/api/pods/delete", any(not_implemented))
        .route("/api/node/cordon", any(not_implemented))
        .route("/api/logs", any(not_implemented))
        .route("/api/describe", any(not_implemented))
        .route("/api/exec", any(not_implemented))
        .route("/api/portforward/*rest", any(not_implemented))
        .route("/api/helm/*rest", any(not_implemented))
        .route("/api/flux/action", any(not_implemented))
        .route("/api/yscale", any(not_implemented))
        .nest_service("/critters", ServeDir::new(&args.critters_dir))
        .fallback(ui::static_handler)
        .layer(CorsLayer::permissive())
        .with_state(state);

    let addr = SocketAddr::from(([127, 0, 0, 1], args.port));
    let listener = tokio::net::TcpListener::bind(addr).await?;
    let url = format!("http://127.0.0.1:{}", args.port);
    println!("kubagachi-native · {url}");
    if args.open {
        let _ = open::that(&url);
    }
    axum::serve(listener, router).await?;
    Ok(())
}

async fn api_snapshot(State(app): State<Arc<AppState>>) -> impl IntoResponse {
    let body = app.snapshot_rx.borrow().clone();
    (
        [(header::CONTENT_TYPE, "application/json")],
        body.as_str().to_owned(),
    )
}

async fn api_stream(
    State(app): State<Arc<AppState>>,
) -> Sse<impl Stream<Item = Result<Event, Infallible>>> {
    // WatchStream yields the current snapshot immediately, then one item per
    // refresher publish — matching the Go server's send-on-connect + fan-out.
    let stream = WatchStream::new(app.snapshot_rx.clone())
        .map(|s: Arc<String>| Ok(Event::default().data(s.as_str())));
    Sse::new(stream).keep_alive(
        KeepAlive::new()
            .interval(Duration::from_secs(25))
            .text("ping"),
    )
}

async fn api_critters(State(app): State<Arc<AppState>>) -> impl IntoResponse {
    // PNG decoding is CPU-bound; keep it off the async workers.
    let list = tokio::task::block_in_place(|| sprites::scan(&app.critters_dir, &app.bounds_cache));
    Json(serde_json::json!({ "states": sprites::STATES, "critters": list }))
}

async fn api_contexts(State(app): State<Arc<AppState>>) -> impl IntoResponse {
    match kube::config::Kubeconfig::read() {
        Ok(kc) => {
            let current = app
                .kube_context
                .clone()
                .or(kc.current_context.clone())
                .unwrap_or_default();
            let contexts = kc
                .contexts
                .iter()
                .map(|nc| wire::WebContext {
                    name: nc.name.clone(),
                    cluster: nc
                        .context
                        .as_ref()
                        .map(|c| c.cluster.clone())
                        .unwrap_or_default(),
                    namespace: nc
                        .context
                        .as_ref()
                        .and_then(|c| c.namespace.clone())
                        .filter(|ns| !ns.is_empty()),
                })
                .collect();
            Json(wire::WebContextList { current, contexts }).into_response()
        }
        Err(err) => (StatusCode::BAD_GATEWAY, err.to_string()).into_response(),
    }
}

async fn not_implemented() -> impl IntoResponse {
    (
        StatusCode::NOT_IMPLEMENTED,
        Json(serde_json::json!({
            "error": "not implemented in kubagachi-native (read-only server)"
        })),
    )
}
