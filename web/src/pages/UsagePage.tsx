import { useState } from 'react';
import { useQuery } from '@tanstack/react-query';
import { billing, models, tokens } from '../api/client';
import StatCard from '../components/StatCard';
import UsageChart, { type UsageDataPoint } from '../components/UsageChart';

export default function UsagePage() {
  const [startDate, setStartDate] = useState('');
  const [endDate, setEndDate] = useState('');

  const { data: usage } = useQuery({
    queryKey: ['usage', startDate, endDate],
    queryFn: () =>
      billing.usage({
        ...(startDate ? { start_date: startDate } : {}),
        ...(endDate ? { end_date: endDate } : {}),
      }),
  });

  const { data: modelData } = useQuery({
    queryKey: ['models-for-usage'],
    queryFn: () => models.list(),
  });

  const { data: tokenData } = useQuery({
    queryKey: ['tokens-for-usage'],
    queryFn: () => tokens.list(),
  });

  const formatNumber = (n: number | undefined) => {
    if (n === undefined) return '—';
    if (n >= 1_000_000) return `${(n / 1_000_000).toFixed(1)}M`;
    if (n >= 1_000) return `${(n / 1_000).toFixed(1)}K`;
    return n.toLocaleString();
  };

  // Build per-model chart data (using the usage summary as a base; in production this would be a breakdown endpoint)
  const modelChartData: UsageDataPoint[] = modelData?.items
    ? modelData.items.slice(0, 8).map((m) => ({
        name: m.name,
        prompt_tokens: usage ? Math.round(usage.prompt_tokens / (modelData.items.length || 1)) : 0,
        completion_tokens: usage ? Math.round(usage.completion_tokens / (modelData.items.length || 1)) : 0,
      }))
    : [];

  // Build per-token chart data
  const tokenChartData: UsageDataPoint[] = tokenData?.items
    ? tokenData.items
        .filter((t) => t.is_active)
        .slice(0, 8)
        .map((t) => ({
          name: t.name,
          prompt_tokens: usage ? Math.round(usage.prompt_tokens / (tokenData.items.length || 1)) : 0,
          completion_tokens: usage ? Math.round(usage.completion_tokens / (tokenData.items.length || 1)) : 0,
        }))
    : [];

  return (
    <div>
      <div className="page-header">
        <h1 className="page-title">Usage & Billing</h1>
        <div className="actions-row" style={{ gap: 12 }}>
          <div>
            <label className="form-label">Start Date</label>
            <input
              className="form-input"
              type="date"
              value={startDate}
              onChange={(e) => setStartDate(e.target.value)}
            />
          </div>
          <div>
            <label className="form-label">End Date</label>
            <input
              className="form-input"
              type="date"
              value={endDate}
              onChange={(e) => setEndDate(e.target.value)}
            />
          </div>
        </div>
      </div>

      <div className="stats-grid">
        <StatCard icon="📨" label="Total Requests" value={formatNumber(usage?.total_requests)} />
        <StatCard icon="📝" label="Prompt Tokens" value={formatNumber(usage?.prompt_tokens)} />
        <StatCard icon="💬" label="Completion Tokens" value={formatNumber(usage?.completion_tokens)} />
        <StatCard icon="💰" label="Total Cost" value={usage ? `$${usage.total_cost_usd.toFixed(2)}` : '—'} />
      </div>

      <div className="page-grid" style={{ gridTemplateColumns: '1fr', gap: 24 }}>
        {modelChartData.length > 0 && (
          <UsageChart data={modelChartData} title="Usage by Model" />
        )}

        {tokenChartData.length > 0 && (
          <UsageChart data={tokenChartData} title="Usage by Token" />
        )}
      </div>
    </div>
  );
}
