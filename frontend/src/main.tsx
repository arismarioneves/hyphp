import React from 'react';
import ReactDOM from 'react-dom/client';
import '@wailsio/runtime';
import './styles/globals.css';
import App from './App';
import { I18nProvider } from './i18n';
import { PlatformProvider } from './lib/platform';

ReactDOM.createRoot(document.getElementById('root') as HTMLElement).render(
  <React.StrictMode>
    <PlatformProvider>
      <I18nProvider>
        <App />
      </I18nProvider>
    </PlatformProvider>
  </React.StrictMode>,
);
