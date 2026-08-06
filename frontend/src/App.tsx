import { Button } from '@/components/ui/button'

function App() {
  return (
    <main className="flex min-h-svh flex-col items-center justify-center gap-2 text-center">
      <h1 className="text-4xl font-semibold tracking-tight">Protean</h1>
      <p className="text-muted-foreground">
        A sandboxed, per-project AI agent workspace.
      </p>
      <Button variant="outline" className="mt-4">
        Get started
      </Button>
    </main>
  )
}

export default App
