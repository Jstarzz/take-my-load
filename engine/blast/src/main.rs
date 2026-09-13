use std::{
    env, process,
    sync::{
        Arc,
        atomic::{AtomicU64, Ordering},
    },
    time::{Duration, Instant},
};

use bytes::Bytes;
use http_body_util::{BodyExt, Empty};
use hyper::{Request, Uri};
use hyper_util::{
    client::legacy::{Client, connect::HttpConnector},
    rt::TokioExecutor,
};
use serde::Serialize;
use tokio::{sync::Semaphore, time::MissedTickBehavior};

const VERSION: &str = env!("CARGO_PKG_VERSION");
const MAX_RPS: u64 = 1_000_000;
const MAX_CONCURRENCY: usize = 1_000_000;

#[derive(Debug, Clone)]
struct RunConfig {
    target: String,
    rps: u64,
    duration_seconds: u64,
    concurrency: usize,
}

#[derive(Debug, Serialize)]
struct Summary {
    engine: &'static str,
    version: &'static str,
    target: String,
    requested_rps: u64,
    duration_ms: u128,
    concurrency: usize,
    scheduled: u64,
    started: u64,
    completed: u64,
    failed: u64,
    backpressured: u64,
    bytes_received: u64,
    actual_rps: f64,
    latency_samples: u64,
    latency_min_us: u64,
    latency_p50_us: u64,
    latency_p95_us: u64,
    latency_p99_us: u64,
    latency_max_us: u64,
    status_1xx: u64,
    status_2xx: u64,
    status_3xx: u64,
    status_4xx: u64,
    status_5xx: u64,
    status_other: u64,
}

struct Stats {
    started: AtomicU64,
    completed: AtomicU64,
    failed: AtomicU64,
    backpressured: AtomicU64,
    bytes_received: AtomicU64,
    status: [AtomicU64; 6],
    latency: LatencyHistogram,
}

impl Stats {
    fn new() -> Self {
        Self {
            started: AtomicU64::new(0),
            completed: AtomicU64::new(0),
            failed: AtomicU64::new(0),
            backpressured: AtomicU64::new(0),
            bytes_received: AtomicU64::new(0),
            status: [const { AtomicU64::new(0) }; 6],
            latency: LatencyHistogram::new(),
        }
    }

    fn record_status(&self, status: u16) {
        let bucket = match status {
            100..=199 => 0,
            200..=299 => 1,
            300..=399 => 2,
            400..=499 => 3,
            500..=599 => 4,
            _ => 5,
        };
        self.status[bucket].fetch_add(1, Ordering::Relaxed);
    }
}

struct LatencyHistogram {
    buckets: [AtomicU64; 64],
    samples: AtomicU64,
    min_us: AtomicU64,
    max_us: AtomicU64,
}

impl LatencyHistogram {
    fn new() -> Self {
        Self {
            buckets: [const { AtomicU64::new(0) }; 64],
            samples: AtomicU64::new(0),
            min_us: AtomicU64::new(u64::MAX),
            max_us: AtomicU64::new(0),
        }
    }

    fn record(&self, duration: Duration) {
        let micros = duration.as_micros().min(u64::MAX as u128) as u64;
        let bucket = latency_bucket(micros);
        self.buckets[bucket].fetch_add(1, Ordering::Relaxed);
        self.samples.fetch_add(1, Ordering::Relaxed);
        self.min_us.fetch_min(micros, Ordering::Relaxed);
        self.max_us.fetch_max(micros, Ordering::Relaxed);
    }

    fn percentile(&self, percentile: f64) -> u64 {
        let samples = self.samples.load(Ordering::Relaxed);
        if samples == 0 {
            return 0;
        }
        let target = ((samples as f64 * percentile).ceil() as u64).max(1);
        let mut seen = 0_u64;
        for (index, bucket) in self.buckets.iter().enumerate() {
            seen = seen.saturating_add(bucket.load(Ordering::Relaxed));
            if seen >= target {
                return latency_bucket_upper_bound(index);
            }
        }
        self.max_us.load(Ordering::Relaxed)
    }

    fn min(&self) -> u64 {
        let value = self.min_us.load(Ordering::Relaxed);
        if value == u64::MAX { 0 } else { value }
    }
}

#[tokio::main(flavor = "multi_thread")]
async fn main() {
    let mut args = env::args();
    let _program = args.next();
    let command = args.next();

    let result = match command.as_deref() {
        Some("--capabilities") => {
            println!(
                "{{\"engine\":\"blast\",\"version\":\"{VERSION}\",\"protocols\":[\"http/1.1\"],\"fixed_rate\":true,\"max_configured_rps\":{MAX_RPS},\"status\":\"alpha\"}}"
            );
            Ok(())
        }
        Some("--version") => {
            println!("tml-blast {VERSION}");
            Ok(())
        }
        Some("run") => match parse_run_config(args) {
            Ok(config) => run(config).await,
            Err(error) => Err(error),
        },
        Some("--help") | Some("-h") | None => {
            print_help();
            Ok(())
        }
        Some(other) => Err(format!("unknown command {other:?}")),
    };

    if let Err(error) = result {
        eprintln!("tml-blast: {error}");
        print_help();
        process::exit(2);
    }
}

async fn run(config: RunConfig) -> Result<(), String> {
    let uri: Uri = config
        .target
        .parse()
        .map_err(|error| format!("invalid target URI: {error}"))?;
    if uri.scheme_str() != Some("http") {
        return Err("blast currently supports explicit http:// targets only".to_string());
    }
    if uri.authority().is_none() {
        return Err("target URI must include a host".to_string());
    }

    let mut connector = HttpConnector::new();
    connector.enforce_http(true);
    connector.set_nodelay(true);
    let client: Client<HttpConnector, Empty<Bytes>> = Client::builder(TokioExecutor::new())
        .pool_max_idle_per_host(config.concurrency)
        .build(connector);

    let stats = Arc::new(Stats::new());
    let semaphore = Arc::new(Semaphore::new(config.concurrency));
    let duration = Duration::from_secs(config.duration_seconds);
    let started_at = Instant::now();
    let mut interval = tokio::time::interval(Duration::from_millis(1));
    interval.set_missed_tick_behavior(MissedTickBehavior::Skip);
    interval.tick().await;

    let mut scheduled = 0_u64;
    while started_at.elapsed() < duration {
        interval.tick().await;
        let elapsed = started_at.elapsed().min(duration);
        let expected = ((elapsed.as_nanos() * config.rps as u128) / 1_000_000_000_u128)
            .min(u64::MAX as u128) as u64;

        while scheduled < expected {
            scheduled += 1;
            let permit = match semaphore.clone().try_acquire_owned() {
                Ok(permit) => permit,
                Err(_) => {
                    stats.backpressured.fetch_add(1, Ordering::Relaxed);
                    continue;
                }
            };

            stats.started.fetch_add(1, Ordering::Relaxed);
            let client = client.clone();
            let uri = uri.clone();
            let stats = stats.clone();
            tokio::spawn(async move {
                let _permit = permit;
                execute_one(client, uri, stats).await;
            });
        }
    }

    let drain = semaphore
        .clone()
        .acquire_many_owned(config.concurrency as u32)
        .await
        .map_err(|_| "request semaphore closed while draining".to_string())?;
    drop(drain);

    let elapsed = started_at.elapsed();
    let completed = stats.completed.load(Ordering::Relaxed);
    let summary = Summary {
        engine: "blast",
        version: VERSION,
        target: config.target,
        requested_rps: config.rps,
        duration_ms: elapsed.as_millis(),
        concurrency: config.concurrency,
        scheduled,
        started: stats.started.load(Ordering::Relaxed),
        completed,
        failed: stats.failed.load(Ordering::Relaxed),
        backpressured: stats.backpressured.load(Ordering::Relaxed),
        bytes_received: stats.bytes_received.load(Ordering::Relaxed),
        actual_rps: if elapsed.is_zero() {
            0.0
        } else {
            completed as f64 / elapsed.as_secs_f64()
        },
        latency_samples: stats.latency.samples.load(Ordering::Relaxed),
        latency_min_us: stats.latency.min(),
        latency_p50_us: stats.latency.percentile(0.50),
        latency_p95_us: stats.latency.percentile(0.95),
        latency_p99_us: stats.latency.percentile(0.99),
        latency_max_us: stats.latency.max_us.load(Ordering::Relaxed),
        status_1xx: stats.status[0].load(Ordering::Relaxed),
        status_2xx: stats.status[1].load(Ordering::Relaxed),
        status_3xx: stats.status[2].load(Ordering::Relaxed),
        status_4xx: stats.status[3].load(Ordering::Relaxed),
        status_5xx: stats.status[4].load(Ordering::Relaxed),
        status_other: stats.status[5].load(Ordering::Relaxed),
    };

    println!(
        "{}",
        serde_json::to_string(&summary).map_err(|error| format!("encode summary: {error}"))?
    );
    Ok(())
}

async fn execute_one(client: Client<HttpConnector, Empty<Bytes>>, uri: Uri, stats: Arc<Stats>) {
    let started = Instant::now();
    let request = match Request::get(uri)
        .header(
            "user-agent",
            concat!("tml-blast/", env!("CARGO_PKG_VERSION")),
        )
        .body(Empty::<Bytes>::new())
    {
        Ok(request) => request,
        Err(_) => {
            stats.failed.fetch_add(1, Ordering::Relaxed);
            stats.latency.record(started.elapsed());
            return;
        }
    };

    match client.request(request).await {
        Ok(response) => {
            stats.record_status(response.status().as_u16());
            match response.into_body().collect().await {
                Ok(body) => {
                    stats
                        .bytes_received
                        .fetch_add(body.to_bytes().len() as u64, Ordering::Relaxed);
                    stats.completed.fetch_add(1, Ordering::Relaxed);
                }
                Err(_) => {
                    stats.failed.fetch_add(1, Ordering::Relaxed);
                }
            }
        }
        Err(_) => {
            stats.failed.fetch_add(1, Ordering::Relaxed);
        }
    }
    stats.latency.record(started.elapsed());
}

fn parse_run_config(mut args: impl Iterator<Item = String>) -> Result<RunConfig, String> {
    let mut target = None;
    let mut rps = None;
    let mut duration_seconds = None;
    let mut concurrency = 1_024_usize;

    while let Some(flag) = args.next() {
        let value = || format!("missing value for {flag}");
        match flag.as_str() {
            "--target" => target = Some(args.next().ok_or_else(value)?),
            "--rps" => {
                rps = Some(
                    args.next()
                        .ok_or_else(value)?
                        .parse::<u64>()
                        .map_err(|_| "--rps must be an integer".to_string())?,
                )
            }
            "--duration-seconds" => {
                duration_seconds = Some(
                    args.next()
                        .ok_or_else(value)?
                        .parse::<u64>()
                        .map_err(|_| "--duration-seconds must be an integer".to_string())?,
                )
            }
            "--concurrency" => {
                concurrency = args
                    .next()
                    .ok_or_else(value)?
                    .parse::<usize>()
                    .map_err(|_| "--concurrency must be an integer".to_string())?;
            }
            other => return Err(format!("unknown run option {other:?}")),
        }
    }

    let rps = rps.ok_or_else(|| "--rps is required".to_string())?;
    if rps == 0 || rps > MAX_RPS {
        return Err(format!("--rps must be between 1 and {MAX_RPS}"));
    }
    let duration_seconds =
        duration_seconds.ok_or_else(|| "--duration-seconds is required".to_string())?;
    if duration_seconds == 0 || duration_seconds > 3_600 {
        return Err("--duration-seconds must be between 1 and 3600".to_string());
    }
    if concurrency == 0 || concurrency > MAX_CONCURRENCY || concurrency > u32::MAX as usize {
        return Err(format!(
            "--concurrency must be between 1 and {MAX_CONCURRENCY}"
        ));
    }

    Ok(RunConfig {
        target: target.ok_or_else(|| "--target is required".to_string())?,
        rps,
        duration_seconds,
        concurrency,
    })
}

fn latency_bucket(micros: u64) -> usize {
    if micros <= 1 {
        0
    } else {
        (63 - micros.leading_zeros()) as usize
    }
}

fn latency_bucket_upper_bound(index: usize) -> u64 {
    if index >= 63 {
        u64::MAX
    } else {
        (1_u64 << (index + 1)) - 1
    }
}

fn print_help() {
    eprintln!("usage:");
    eprintln!("  tml-blast --capabilities");
    eprintln!("  tml-blast --version");
    eprintln!(
        "  tml-blast run --target http://HOST/PATH --rps N --duration-seconds N [--concurrency N]"
    );
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn parses_run_configuration() {
        let config = parse_run_config(
            [
                "--target",
                "http://127.0.0.1:8080/healthz",
                "--rps",
                "100000",
                "--duration-seconds",
                "30",
                "--concurrency",
                "4096",
            ]
            .into_iter()
            .map(str::to_string),
        )
        .expect("configuration should parse");

        assert_eq!(config.rps, 100_000);
        assert_eq!(config.duration_seconds, 30);
        assert_eq!(config.concurrency, 4_096);
    }

    #[test]
    fn rejects_rate_above_platform_ceiling() {
        let error = parse_run_config(
            [
                "--target",
                "http://127.0.0.1",
                "--rps",
                "1000001",
                "--duration-seconds",
                "1",
            ]
            .into_iter()
            .map(str::to_string),
        )
        .expect_err("rate should be rejected");
        assert!(error.contains("between 1 and 1000000"));
    }

    #[test]
    fn latency_histogram_orders_percentiles() {
        let histogram = LatencyHistogram::new();
        for micros in [10_u64, 20, 30, 40, 50, 1_000] {
            histogram.record(Duration::from_micros(micros));
        }
        assert!(histogram.percentile(0.50) <= histogram.percentile(0.95));
        assert!(histogram.percentile(0.95) <= histogram.percentile(0.99));
        assert_eq!(histogram.min(), 10);
    }
}
