import { useState, useEffect } from 'react'
import { fetchConfig, fetchSetupStatus, fetchBackgroundArt } from './lib/api'
import Wizard from './components/Wizard'
import Settings from './components/Settings'

export default function App() {
  const [view, setView] = useState(null)
  const [config, setConfig] = useState({})
  const [envSources, setEnvSources] = useState({})
  const [bgUrl, setBgUrl] = useState(null)
  const [bgLoaded, setBgLoaded] = useState(false)

  useEffect(() => {
    Promise.all([
      fetchSetupStatus(),
      fetchBackgroundArt(),
    ]).then(([status, artUrl]) => {
      if (artUrl) setBgUrl(artUrl)
      loadAppData()
    })
  }, [])

  async function loadAppData() {
    const { values, sources } = await fetchConfig()
    setConfig(values)
    setEnvSources(sources || {})
    const nextView = values.WIZARD_COMPLETE === 'true' ? 'settings' : 'wizard'
    setView(nextView)
  }

  if (view === null) return <div className="min-h-screen bg-bg" />

  if (view === 'wizard') {
    return (
      <Wizard
        config={config}
        envSources={envSources}
        bgUrl={bgUrl}
        bgLoaded={bgLoaded}
        onBgLoad={() => setBgLoaded(true)}
        onComplete={() => {
          fetchConfig().then(({ values, sources }) => {
            setConfig(values)
            setEnvSources(sources || {})
            setView('settings')
          })
        }}
      />
    )
  }

  return (
    <Settings
      onWizard={() => setView('wizard')}
    />
  )
}
