import { useState, useEffect, useRef } from 'react'
import axios from 'axios'
import { Package, RefreshCw, ShoppingCart, CheckCircle, AlertCircle, History } from 'lucide-react'
import './index.css'

const API_BASE = 'http://localhost:8080/api/v1'
const ITEMS = ['item_4021', 'item_8888', 'item_9999']

function App() {
  const [selectedItem, setSelectedItem] = useState(ITEMS[0])
  const [inventory, setInventory] = useState({
    total_stock: 0,
    reserved_stock: 0,
    available_stock: 0
  })
  const [history, setHistory] = useState([])
  const [loading, setLoading] = useState(false)
  const [error, setError] = useState(null)

  // Reservation state
  const [userId, setUserId] = useState(`usr_${Math.floor(Math.random() * 10000)}`)
  const [quantity, setQuantity] = useState(1)
  const [reserving, setReserving] = useState(false)

  // Active reservation state
  const [activeReservation, setActiveReservation] = useState(null)
  const [timeLeft, setTimeLeft] = useState(0)
  const [confirming, setConfirming] = useState(false)
  const [successMsg, setSuccessMsg] = useState(null)

  const timerRef = useRef(null)

  const fetchData = async () => {
    setLoading(true)
    try {
      const res = await axios.get(`${API_BASE}/inventory/stock?item_id=${selectedItem}`)
      setInventory(res.data)
      
      const histRes = await axios.get(`${API_BASE}/inventory/history?item_id=${selectedItem}`)
      setHistory(histRes.data || [])
      
      setError(null)
    } catch (err) {
      setError('Failed to fetch data.')
      console.error(err)
    } finally {
      setLoading(false)
    }
  }

  useEffect(() => {
    fetchData()
    // Poll every 5 seconds
    const interval = setInterval(fetchData, 5000)
    return () => clearInterval(interval)
  }, [selectedItem])

  useEffect(() => {
    if (activeReservation && timeLeft > 0) {
      timerRef.current = setInterval(() => {
        setTimeLeft((prev) => {
          if (prev <= 1) {
            clearInterval(timerRef.current)
            setActiveReservation(null)
            setError('Reservation expired.')
            fetchData()
            return 0
          }
          return prev - 1
        })
      }, 1000)
    } else if (timeLeft <= 0 && activeReservation) {
      setActiveReservation(null)
    }

    return () => {
      if (timerRef.current) clearInterval(timerRef.current)
    }
  }, [activeReservation])

  const handleReserve = async (e) => {
    e.preventDefault()
    if (quantity <= 0) return

    setReserving(true)
    setError(null)
    setSuccessMsg(null)

    try {
      const res = await axios.post(`${API_BASE}/inventory/reserve`, {
        user_id: userId,
        item_id: selectedItem,
        quantity: parseInt(quantity)
      })

      const expiresAt = new Date(res.data.expires_at).getTime()
      const now = new Date().getTime()
      const secondsLeft = Math.max(0, Math.floor((expiresAt - now) / 1000))

      setActiveReservation(res.data)
      setTimeLeft(secondsLeft)
      fetchData()
    } catch (err) {
      setError(err.response?.data?.message || 'Failed to reserve stock.')
    } finally {
      setReserving(false)
    }
  }

  const handleConfirm = async () => {
    if (!activeReservation) return

    setConfirming(true)
    setError(null)

    try {
      await axios.post(`${API_BASE}/inventory/confirm`, {
        reservation_id: activeReservation.reservation_id
      })

      setSuccessMsg(`Order confirmed! ID: ${activeReservation.reservation_id}`)
      setActiveReservation(null)
      if (timerRef.current) clearInterval(timerRef.current)
      fetchData()
    } catch (err) {
      setError(err.response?.data?.message || 'Failed to confirm order.')
      setActiveReservation(null) 
      fetchData()
    } finally {
      setConfirming(false)
    }
  }

  const formatTime = (seconds) => {
    const m = Math.floor(seconds / 60).toString().padStart(2, '0')
    const s = (seconds % 60).toString().padStart(2, '0')
    return `${m}:${s}`
  }

  return (
    <div>
      <div className="header">
        <h1>Fullstack Take Home Assignment</h1>
        <p style={{ color: '#94a3b8' }}>High-Throughput Inventory Reservation System & Mini Dashboard</p>
      </div>

      <div style={{ maxWidth: '800px', margin: '0 auto 2rem auto', background: '#fff', padding: '1.5rem', borderRadius: '12px', border: '1px solid #e2e8f0', boxShadow: '0 4px 6px -1px rgba(0,0,0,0.05)' }}>
        <label className="form-label">Select Product to View</label>
        <select 
          className="form-input" 
          value={selectedItem} 
          onChange={(e) => setSelectedItem(e.target.value)}
          style={{ width: '100%', cursor: 'pointer' }}
        >
          {ITEMS.map(item => (
            <option key={item} value={item}>{item}</option>
          ))}
        </select>
      </div>

      <div className="app-container">
        {/* Inventory Dashboard */}
        <div className="card">
          <h2 className="card-title">
            <Package size={24} color="#0ea5e9" />
            Live Tracker: {selectedItem}
          </h2>

          <div className="stat-grid">
            <div className="stat-box">
              <p className="stat-value">{inventory.total_stock}</p>
              <p className="stat-label">Total Stock</p>
            </div>
            <div className="stat-box">
              <p className="stat-value" style={{ color: '#f59e0b' }}>{inventory.reserved_stock}</p>
              <p className="stat-label">Reserved</p>
            </div>
            <div className="stat-box">
              <p className="stat-value" style={{ color: '#10b981' }}>{inventory.available_stock}</p>
              <p className="stat-label">Available</p>
            </div>
          </div>

          <button
            className="btn"
            onClick={fetchData}
            disabled={loading}
            style={{ marginTop: '1.5rem', background: '#f1f5f9', color: '#0f172a', border: '1px solid #e2e8f0' }}
          >
            <RefreshCw size={18} className={loading ? 'spin' : ''} />
            {loading ? 'Refreshing...' : 'Refresh Status'}
          </button>
        </div>

        {/* Reservation / Checkout Area */}
        <div className="card">
          <h2 className="card-title">
            <ShoppingCart size={24} color="#0ea5e9" />
            {activeReservation ? 'Confirm Order' : 'Reserve Stock'}
          </h2>

          {error && (
            <div className="alert alert-error">
              <AlertCircle size={18} />
              {error}
            </div>
          )}

          {successMsg && (
            <div className="alert alert-success">
              <CheckCircle size={18} />
              {successMsg}
            </div>
          )}

          {!activeReservation ? (
            <form onSubmit={handleReserve}>
              <div className="form-group">
                <label className="form-label">User ID</label>
                <input
                  type="text"
                  className="form-input"
                  value={userId}
                  onChange={(e) => setUserId(e.target.value)}
                  required
                />
              </div>
              <div className="form-group">
                <label className="form-label">Quantity</label>
                <input
                  type="number"
                  className="form-input"
                  min="1"
                  max={inventory.available_stock}
                  value={quantity}
                  onChange={(e) => setQuantity(e.target.value)}
                  required
                />
              </div>
              <button
                type="submit"
                className="btn btn-primary"
                disabled={reserving || inventory.available_stock < 1}
              >
                {reserving ? 'Reserving...' : 'Request Reservation'}
              </button>
            </form>
          ) : (
            <div>
              <p style={{ textAlign: 'center', marginBottom: '1rem', color: '#64748b' }}>
                Your stock is reserved! Please confirm before the timer runs out.
              </p>

              <div className="countdown-box">
                <div className="countdown-time">
                  {formatTime(timeLeft)}
                </div>
                <p className="stat-label" style={{ marginTop: '0.5rem' }}>Time Remaining</p>
              </div>

              <div style={{ background: '#f8fafc', border: '1px solid #e2e8f0', padding: '1rem', borderRadius: '8px', marginBottom: '1.5rem' }}>
                <div style={{ display: 'flex', justifyContent: 'space-between', marginBottom: '0.5rem' }}>
                  <span style={{ color: '#64748b' }}>Item ID:</span>
                  <span style={{ fontWeight: 600, color: '#0f172a' }}>{activeReservation.item_id}</span>
                </div>
                <div style={{ display: 'flex', justifyContent: 'space-between' }}>
                  <span style={{ color: '#64748b' }}>Quantity:</span>
                  <span style={{ fontWeight: 600, color: '#0f172a' }}>{activeReservation.quantity}</span>
                </div>
              </div>

              <button
                className="btn btn-success"
                onClick={handleConfirm}
                disabled={confirming}
              >
                <CheckCircle size={18} />
                {confirming ? 'Confirming...' : 'Confirm Purchase'}
              </button>
            </div>
          )}
        </div>
      </div>
      
      {/* Transaction History */}
      <div style={{ maxWidth: '800px', margin: '2rem auto', background: '#fff', padding: '1.5rem', borderRadius: '12px', border: '1px solid #e2e8f0', boxShadow: '0 4px 6px -1px rgba(0,0,0,0.05)' }}>
        <h2 className="card-title" style={{ marginBottom: '1rem' }}>
            <History size={24} color="#0ea5e9" />
            Transaction History
        </h2>
        {history.length === 0 ? (
          <p style={{ color: '#64748b', textAlign: 'center' }}>No transactions found for {selectedItem}.</p>
        ) : (
          <div style={{ overflowX: 'auto' }}>
            <table className="history-table">
              <thead>
                <tr>
                  <th>Reservation ID</th>
                  <th>User ID</th>
                  <th>Quantity</th>
                  <th>Status</th>
                  <th>Confirmed At</th>
                </tr>
              </thead>
              <tbody>
                {history.map(row => (
                  <tr key={row.reservation_id}>
                    <td>{row.reservation_id}</td>
                    <td>{row.user_id}</td>
                    <td>{row.quantity}</td>
                    <td>
                      <span className={`status-badge status-${row.status}`}>
                        {row.status}
                      </span>
                    </td>
                    <td>{row.confirmed_at ? new Date(row.confirmed_at).toLocaleString() : '-'}</td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        )}
      </div>
    </div>
  )
}

export default App
