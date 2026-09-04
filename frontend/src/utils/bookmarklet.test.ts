import { describe, it, expect } from 'bun:test';
import { generateBookmarkletCode, generateInlineBookmarkletCode, type BookmarkletOptions } from './bookmarklet';

describe('bookmarklet generator', () => {
  it('formats valid javascript: URI wrapped in IIFE', () => {
    const code = generateBookmarkletCode({ serverUrl: 'http://localhost:8080' });
    expect(code.startsWith('javascript:(function(){')).toBe(true);
    expect(code.endsWith('})();')).toBe(true);
    // Should be a single line without newlines suitable for href attribute
    expect(code.includes('\n')).toBe(false);
  });

  it('injects window.__READR_SERVER_URL__ correctly with escaped server URL and trailing slash trimmed', () => {
    const code = generateBookmarkletCode({ serverUrl: 'https://readr.example.com/' });
    expect(code).toContain('window.__READR_SERVER_URL__="https://readr.example.com";');
  });

  it('injects window.__READR_API_KEY__ when provided', () => {
    const code = generateBookmarkletCode({
      serverUrl: 'http://localhost:8080',
      apiKey: 'rdr_live_secret123',
    });
    expect(code).toContain('window.__READR_API_KEY__="rdr_live_secret123";');
  });

  it('omits or clears window.__READR_API_KEY__ when empty or not provided', () => {
    const codeNoToken = generateBookmarkletCode({ serverUrl: 'http://localhost:8080' });
    expect(codeNoToken).toContain('window.__READR_API_KEY__="";');

    const codeEmptyToken = generateBookmarkletCode({
      serverUrl: 'http://localhost:8080',
      apiKey: '',
    });
    expect(codeEmptyToken).toContain('window.__READR_API_KEY__="";');
  });

  it('dynamically loads <serverUrl>/bookmarklet.js via document.createElement', () => {
    const code = generateBookmarkletCode({ serverUrl: 'http://localhost:8080' });
    expect(code).toContain("document.createElement('script')");
    expect(code).toContain('/bookmarklet.js?_=');
    expect(code).toContain('document.body.appendChild(s)');
  });

  it('appends cachebuster query parameter to script source so updates load immediately', () => {
    const code = generateBookmarkletCode({ serverUrl: 'http://localhost:8080' });
    expect(code).toContain('+new Date().getTime()');
  });

  it('handles quotes and special characters in serverUrl and apiKey safely', () => {
    const code = generateBookmarkletCode({
      serverUrl: 'https://readr.example.com/test"path',
      apiKey: 'key"with\'quotes\\and/slashes',
    });
    // JSON.stringify handles escaping quotes safely
    expect(code).toContain('window.__READR_SERVER_URL__="https://readr.example.com/test\\"path";');
    expect(code).toContain('window.__READR_API_KEY__="key\\"with\'quotes\\\\and/slashes";');
  });

  describe('generateInlineBookmarkletCode', () => {
    it('generates a valid standalone inline loader javascript: URI', () => {
      const code = generateInlineBookmarkletCode({
        serverUrl: 'http://localhost:8080',
        authToken: 'token-abc',
      });
      expect(code.startsWith('javascript:(function(){')).toBe(true);
      expect(code.endsWith('})();')).toBe(true);
      expect(code.includes('\n')).toBe(false);
      expect(code).toContain('window.__READR_SERVER_URL__="http://localhost:8080";');
      expect(code).toContain('window.__READR_AUTH_TOKEN__="token-abc";');
      expect(code).toContain('/bookmarklet.js');
    });
  });
});
