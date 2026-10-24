import { createRoot } from 'react-dom/client'
import App from './App'
import './index.css'

// StrictMode is intentionally not used: its double-invoked effects would open
// every SSH session twice in development.
createRoot(document.getElementById('root')!).render(<App />)
