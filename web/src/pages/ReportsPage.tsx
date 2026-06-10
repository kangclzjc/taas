import { useQuery } from "@tanstack/react-query";
import StatCard from "../components/StatCard";

interface ReportManifest {
  reports: ReportEntry[];
}

interface ReportEntry {
  id: string;
  title: string;
  model: string;
  backend: string;
  platform: string;
  status: string;
  generated_at: string;
  url: string;
  summary_url: string;
  headline?: {
    sla_goodput_output_tokens_per_second?: number;
    cost_per_1m_input_tokens_usd?: number;
    cost_per_1m_output_tokens_usd?: number;
    suggested_input_price_per_1m_tokens_usd?: number;
    suggested_output_price_per_1m_tokens_usd?: number;
    suggested_mixed_price_per_1m_tokens_usd?: number;
    required_gpus_with_ha?: number;
    observed_average_gpu_count?: number;
    capacity_surplus_gpus?: number;
    perf_per_dollar?: number;
    baseline_goodput_delta_pct?: number;
  };
}

const formatNumber = (value: number | undefined, digits = 1) => {
  if (value === undefined || !Number.isFinite(value)) return "—";
  if (Math.abs(value) >= 1_000_000)
    return `${(value / 1_000_000).toFixed(digits)}M`;
  if (Math.abs(value) >= 1_000) return `${(value / 1_000).toFixed(digits)}K`;
  return value.toFixed(digits);
};

const formatMoney = (value: number | undefined) => {
  if (value === undefined || !Number.isFinite(value)) return "—";
  return `$${value.toFixed(4)}`;
};

const formatGpuGap = (value: number | undefined) => {
  if (value === undefined || !Number.isFinite(value)) return "—";
  return `${value >= 0 ? "+" : ""}${value.toFixed(1)} GPUs`;
};

async function fetchManifest(): Promise<ReportManifest> {
  const res = await fetch("/report-manifest.json");
  if (!res.ok) {
    throw new Error(`Failed to load report manifest (${res.status})`);
  }
  return res.json();
}

export default function ReportsPage() {
  const { data, isLoading, error } = useQuery({
    queryKey: ["report-manifest"],
    queryFn: fetchManifest,
  });

  const reports = data?.reports ?? [];
  const first = reports[0];

  return (
    <div>
      <div className="page-header">
        <div>
          <h1 className="page-title">Reports</h1>
          <p style={{ color: "var(--text-secondary)", marginTop: 4 }}>
            Token economics artifacts generated from offline validation runs.
          </p>
        </div>
      </div>

      <div className="stats-grid">
        <StatCard
          icon="⚡"
          label="SLA Goodput"
          value={
            first
              ? `${formatNumber(first.headline?.sla_goodput_output_tokens_per_second)} tok/s`
              : "—"
          }
        />
        <StatCard
          icon="💵"
          label="Observed Cost / 1M Input"
          value={
            first
              ? formatMoney(first.headline?.cost_per_1m_input_tokens_usd)
              : "—"
          }
        />
        <StatCard
          icon="💵"
          label="Observed Cost / 1M Output"
          value={
            first
              ? formatMoney(first.headline?.cost_per_1m_output_tokens_usd)
              : "—"
          }
        />
        <StatCard
          icon="🏷️"
          label="Suggested / 1M Input"
          value={
            first
              ? formatMoney(
                  first.headline?.suggested_input_price_per_1m_tokens_usd,
                )
              : "—"
          }
        />
        <StatCard
          icon="🏷️"
          label="Suggested / 1M Output"
          value={
            first
              ? formatMoney(
                  first.headline?.suggested_output_price_per_1m_tokens_usd,
                )
              : "—"
          }
        />
        <StatCard
          icon="↗"
          label="Capacity Headroom"
          value={
            first ? formatGpuGap(first.headline?.capacity_surplus_gpus) : "—"
          }
        />
      </div>

      <div className="card">
        <div className="card-header">
          <div className="card-title">Token Economics Reports</div>
          <span className="badge badge-info">
            {reports.length} artifact{reports.length === 1 ? "" : "s"}
          </span>
        </div>

        {isLoading && (
          <div style={{ color: "var(--text-secondary)" }}>
            Loading reports...
          </div>
        )}
        {error && (
          <div className="badge badge-danger">
            {error instanceof Error ? error.message : "Failed to load reports"}
          </div>
        )}

        {!isLoading && !error && reports.length === 0 && (
          <div style={{ color: "var(--text-secondary)" }}>
            No reports are available yet.
          </div>
        )}

        {reports.length > 0 && (
          <div className="table-wrapper">
            <table>
              <thead>
                <tr>
                  <th>Report</th>
                  <th>Model</th>
                  <th>Backend</th>
                  <th>Status</th>
                  <th>Goodput</th>
                  <th>Observed Cost / 1M Input</th>
                  <th>Observed Cost / 1M Output</th>
                  <th>Suggested / 1M Input</th>
                  <th>Suggested / 1M Output</th>
                  <th>Capacity</th>
                  <th></th>
                </tr>
              </thead>
              <tbody>
                {reports.map((report) => (
                  <tr key={report.id}>
                    <td>
                      <div style={{ fontWeight: 600 }}>{report.title}</div>
                      <div style={{ color: "var(--text-muted)", fontSize: 12 }}>
                        {report.id}
                      </div>
                    </td>
                    <td>{report.model}</td>
                    <td>{report.backend}</td>
                    <td>
                      <span
                        className={
                          report.status === "synthetic"
                            ? "badge badge-warning"
                            : "badge badge-success"
                        }
                      >
                        {report.status}
                      </span>
                    </td>
                    <td>
                      {formatNumber(
                        report.headline?.sla_goodput_output_tokens_per_second,
                      )}{" "}
                      tok/s
                    </td>
                    <td>
                      {formatMoney(
                        report.headline?.cost_per_1m_input_tokens_usd,
                      )}
                    </td>
                    <td>
                      {formatMoney(
                        report.headline?.cost_per_1m_output_tokens_usd,
                      )}
                    </td>
                    <td>
                      {formatMoney(
                        report.headline
                          ?.suggested_input_price_per_1m_tokens_usd,
                      )}
                    </td>
                    <td>
                      {formatMoney(
                        report.headline
                          ?.suggested_output_price_per_1m_tokens_usd,
                      )}
                    </td>
                    <td>
                      {formatGpuGap(report.headline?.capacity_surplus_gpus)}
                    </td>
                    <td style={{ textAlign: "right" }}>
                      <a className="btn btn-primary btn-sm" href={report.url}>
                        Open Report
                      </a>
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        )}
      </div>
    </div>
  );
}
