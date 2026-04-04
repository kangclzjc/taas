import type { ReactNode } from 'react';

interface EmptyStateProps {
  icon: string;
  title: string;
  description?: string;
  action?: ReactNode;
}

export default function EmptyState({ icon, title, description, action }: EmptyStateProps) {
  return (
    <div className="empty-state-component">
      <div className="empty-state-component-icon">{icon}</div>
      <h3 className="empty-state-component-title">{title}</h3>
      {description && <p className="empty-state-component-desc">{description}</p>}
      {action && <div className="empty-state-component-action">{action}</div>}
    </div>
  );
}
