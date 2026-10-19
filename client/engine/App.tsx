import { useEffect, useRef } from 'preact/hooks';
import { PixiRenderer, helloWorld } from './pixi-renderer';

export function App() {
  const host = useRef<HTMLDivElement>(null);
  useEffect(() => {
    const el = host.current;
    if (!el) return;
    const renderer = new PixiRenderer();
    let disposed = false;
    void renderer.init(el).then(() => {
      if (disposed) {
        renderer.destroy();
        return;
      }
      renderer.render(helloWorld());
    });
    return () => {
      disposed = true;
    };
  }, []);
  return <div ref={host} style={{ width: '100%', height: '100%' }} />;
}
