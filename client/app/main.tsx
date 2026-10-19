import { render } from 'preact';
import { createBrowserAdapter } from '../platform';
import { Shell } from './Shell';

const params = new URLSearchParams(location.search);
const lang = params.get('lang');
const root = document.getElementById('app');
if (root) {
  render(
    <Shell
      platform={createBrowserAdapter()}
      // Пусто — тот же origin (dev: vite proxy /v1). Прод/стейджинг: VITE_API_URL при сборке.
      baseUrl={(import.meta.env.VITE_API_URL as string | undefined) ?? ''}
      contentVersion={import.meta.env.VITE_CONTENT_VERSION as string | undefined}
      lang={lang === 'ru' || lang === 'en' ? lang : undefined}
      map={params.get('map') ?? undefined}
    />,
    root,
  );
}
