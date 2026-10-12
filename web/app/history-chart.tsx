type HistoryChartProps = {
  label: string;
  values: number[];
  color: string;
  max?: number;
};

export function HistoryChart({ label, values, color, max }: HistoryChartProps) {
  if (values.length === 0) return <p className="empty-chart">No history yet</p>;
  const ceiling = max ?? Math.max(1, ...values) * 1.15;
  const points = values.map((value, index) => {
    const x = values.length === 1 ? 0 : (index / (values.length - 1)) * 100;
    const y = 40 - (Math.max(0, Math.min(value, ceiling)) / ceiling) * 36;
    return `${x.toFixed(2)},${y.toFixed(2)}`;
  }).join(" ");
  return (
    <svg className="history-chart" viewBox="0 0 100 44" preserveAspectRatio="none" role="img" aria-label={`${label} trend, ${values.length} samples`}>
      <line x1="0" y1="40" x2="100" y2="40" stroke="#dce7e5" strokeWidth="0.5" />
      <polyline points={points} fill="none" stroke={color} strokeWidth="1.8" strokeLinecap="round" strokeLinejoin="round" vectorEffect="non-scaling-stroke" />
    </svg>
  );
}
