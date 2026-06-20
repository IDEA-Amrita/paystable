import { LineChart, Line, ResponsiveContainer } from 'recharts'

export default function Sparkline({ data, color = '#22d3a5', width = 120, height = 40 }) {
  const chartData = data.map((value, index) => ({ index, value }))

  return (
    <div style={{ width, height }}>
      <ResponsiveContainer width="100%" height="100%">
        <LineChart data={chartData}>
          <Line
            type="monotone"
            dataKey="value"
            stroke={color}
            strokeWidth={1.5}
            dot={false}
            isAnimationActive={false}
          />
        </LineChart>
      </ResponsiveContainer>
    </div>
  )
}
