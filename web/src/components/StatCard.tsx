interface StatCardProps {
  icon: string;
  label: string;
  value: string | number;
}

export default function StatCard({ icon, label, value }: StatCardProps) {
  return (
    <div className="stat-card">
      <div className="stat-card-icon">{icon}</div>
      <div className="stat-card-body">
        <div className="stat-card-label">{label}</div>
        <div className="stat-card-value">{value}</div>
      </div>
    </div>
  );
}
