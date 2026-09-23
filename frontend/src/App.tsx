function App() {
  return (
    <main className="h-full bg-bg-app p-6 text-fg">
      <h1 className="font-mono text-lg">HyPHP</h1>
      <div className="mt-4 rounded-card border border-border bg-bg-card p-4 text-fg-muted">
        tokens ok · <span className="text-ok">ok</span> · <span className="text-warn">warn</span> ·{' '}
        <span className="text-err">err</span>
      </div>
    </main>
  );
}

export default App;
