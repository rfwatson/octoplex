export function joinUrl(base: string, ...paths: string[]): string {
  let url = new URL(base);

  for (const path of paths) {
    const cleanPath = path.replace(/^\/+/, '');
    url.pathname = url.pathname.replace(/\/$/, '') + '/' + cleanPath;
  }

  return url.toString();
}

const serverAddrKey = 'serverAddr';

export function setServerAddr(url: string) {
  window.localStorage.setItem(serverAddrKey, url);
}

export function getServerAddr(): string | undefined {
  return window.localStorage.getItem(serverAddrKey) ?? undefined;
}
