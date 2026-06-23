import Sidebar from './Sidebar'

export default function Shell({ children }) {
  return (
    <div className="min-h-screen bg-bg-base">
      <Sidebar />
      <main className="ml-[220px] p-6 min-h-screen">
        {children}
      </main>
    </div>
  )
}
