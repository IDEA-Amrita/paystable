import { BrowserRouter, Routes, Route, Navigate } from 'react-router-dom'
import Shell from './components/layout/Shell'
import Health from './pages/Health'
import Transactions from './pages/Transactions'
import Mismatches from './pages/Mismatches'

export default function App() {
  return (
    <BrowserRouter basename="/dashboard">
      <Shell>
        <Routes>
          <Route index element={<Navigate to="/health" replace />} />
          <Route path="/health" element={<Health />} />
          <Route path="/transactions" element={<Transactions />} />
          <Route path="/mismatches" element={<Mismatches />} />
        </Routes>
      </Shell>
    </BrowserRouter>
  )
}
