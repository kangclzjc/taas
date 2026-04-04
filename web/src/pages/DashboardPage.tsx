import { useQuery } from '@tanstack/react-query';
import { billing, models, tokens } from '../api/client';
import StatCard from '../components/StatCard';
import UsageChart, { type UsageDataPoint } from '../components/UsageChart';

export default function DashboardPage() {
  const { data: usage } = useQuery({
    queryKey: ['usage-summary'],
    queryFn: () => billing.usage(),
  });

  const { data: modelData } = useQuery({
    queryKey: ['models'],
    queryFn: () => models.list(),
  });

  const { data: tokenData } = useQuery({
    queryKey: ['tokens'],
    queryFn: () => tokens.list(),
  });

  const formatNumber = (n: number | undefined) => {
    if (n === undefined) return '—';
    if (n >= 1_000_000) return `${(n / 1_000_000).toFixed(1)}M`;
    if (n >= 1_000) return `${(n / 1_000).toFixed(1)}K`;
    return n.toLocaleString();
  };

  const formatCost = (n: number | undefined) => {
    if (n === undefined) return '—';
    return `$${n.toFixed(2)}`;
  };

  // Generate mock chart data from summary (in production this would come from a timeseries endpoint)
  const chartData: UsageDataPoint[] = usage
    ? [
        { name: 'Current Period', prompt_tokens: usage.prompt_tokens, completion_tokens: usage.completion_tokens },
      ]
    : [];

  return (
    <div>
      <div className="page-header">
        <h1 className="page-title">Dashboard</h1>
      </div>

      <div className="stats-grid">
        <StatCard icon="📨" label="Total Requests" value={formatNumber(usage?.total_requests)} />
        <StatCard icon="🔤" label="Total Tokens" value={formatNumber(usage?.total_tokens)} />
        <StatCard icon="💰" label="Cost (Period)" value={formatCost(usage?.total_cost_usd)} />
        <StatCard icon="🤖" label="Models" value={modelData?.total ?? '—'} />
        <StatCard icon="🔑" label="Active Tokens" value={
          tokenData?.items?.filter((t) => t.is_active).length ?? '—'
        } />
      </div>

      {chartData.length > 0 && (
        <UsageChart data={chartData} title="Token Usage — Current Period" />
      )}
    </div>
  );
}
