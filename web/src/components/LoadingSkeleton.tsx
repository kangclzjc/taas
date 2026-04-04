export function SkeletonCard() {
  return (
    <div className="skeleton-card">
      <div className="skeleton-icon skeleton-pulse" />
      <div className="skeleton-card-body">
        <div className="skeleton-line skeleton-line-sm skeleton-pulse" />
        <div className="skeleton-line skeleton-line-lg skeleton-pulse" />
      </div>
    </div>
  );
}

export function SkeletonTable({ rows = 5, cols = 5 }: { rows?: number; cols?: number }) {
  return (
    <div className="skeleton-table">
      <div className="skeleton-table-header">
        {Array.from({ length: cols }).map((_, i) => (
          <div key={i} className="skeleton-line skeleton-line-md skeleton-pulse" />
        ))}
      </div>
      {Array.from({ length: rows }).map((_, r) => (
        <div key={r} className="skeleton-table-row">
          {Array.from({ length: cols }).map((_, c) => (
            <div key={c} className="skeleton-line skeleton-line-md skeleton-pulse" />
          ))}
        </div>
      ))}
    </div>
  );
}

export function SkeletonPage() {
  return (
    <div className="skeleton-page">
      <div className="skeleton-line skeleton-line-title skeleton-pulse" style={{ marginBottom: 24 }} />
      <div className="stats-grid">
        <SkeletonCard />
        <SkeletonCard />
        <SkeletonCard />
        <SkeletonCard />
      </div>
      <div className="card" style={{ padding: 20 }}>
        <SkeletonTable rows={4} cols={6} />
      </div>
    </div>
  );
}
