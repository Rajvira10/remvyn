import { createRoot } from 'react-dom/client'
import App from './App'
import './index.css'
import { listenForFileDrops } from './lib/fileDrop'

// StrictMode is intentionally not used: its double-invoked effects would open
// every SSH session twice in development.
listenForFileDrops()
createRoot(document.getElementById('root')!).render(<App />)
