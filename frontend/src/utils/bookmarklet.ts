export interface BookmarkletOptions {
  serverUrl: string;
  authToken?: string;
}

/**
 * Generates a clean, single-line `javascript:(function(){...})();` bookmarklet URL
 * that dynamically loads the Readr bookmarklet script from the server with cachebusting
 * and injects the necessary configuration globals (__READR_SERVER_URL__, __READR_AUTH_TOKEN__).
 */
export function generateBookmarkletCode(options: BookmarkletOptions): string {
  const normalizedServerUrl = (options.serverUrl || '').trim().replace(/\/+$/, '');
  const normalizedToken = (options.authToken || '').trim();

  const serverUrlJson = JSON.stringify(normalizedServerUrl);
  const tokenJson = JSON.stringify(normalizedToken);

  return `javascript:(function(){window.__READR_SERVER_URL__=${serverUrlJson};window.__READR_AUTH_TOKEN__=${tokenJson};var s=document.createElement('script');s.src=${serverUrlJson}+'/bookmarklet.js?_='+new Date().getTime();document.body.appendChild(s);})();`;
}

/**
 * Generates standalone inline loader bookmarklet code.
 */
export function generateInlineBookmarkletCode(options: BookmarkletOptions): string {
  return generateBookmarkletCode(options);
}
