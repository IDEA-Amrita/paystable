import { BrowserRouter, Routes, Route, Navigate } from 'react-router-dom'
import Shell from './components/layout/Shell'
import Overview from './pages/Overview'
import Transactions from './pages/Transactions'
import Mismatches from './pages/Mismatches'
import Delivery from './pages/Delivery'
import Config from './pages/Config'

export default function App() {
  return (
    <BrowserRouter basename="/dashboard">
      <Shell>
        <Routes>
          <Route index element={<Navigate to="/overview" replace />} />
          <Route path="/overview" element={<Overview />} />
          <Route path="/transactions" element={<Transactions />} />
          <Route path="/mismatches" element={<Mismatches />} />
          <Route path="/delivery" element={<Delivery />} />
          <Route path="/config" element={<Config />} />
        </Routes>
      </Shell>
    </BrowserRouter>
  )
}
