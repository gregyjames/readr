import { describe, it, expect } from 'bun:test';
import { isMoc, mocRingSegments } from './moc';

describe('isMoc', () => {
  it('detects hubs by the surviving title prefix', () => {
    expect(isMoc('MOC - Kubernetes', 'k8s')).toBe(true);
    expect(isMoc('moc: Azure', '')).toBe(true);
    expect(isMoc('MOC', null)).toBe(true);
  });

  it('detects freshly-created hubs by tag', () => {
    expect(isMoc('Kubernetes Overview', 'moc, k8s')).toBe(true);
    expect(isMoc('Kubernetes Overview', ['moc', 'k8s'])).toBe(true);
  });

  it('leaves plain notes alone', () => {
    expect(isMoc('Mocking in Go', 'testing')).toBe(false);
    expect(isMoc('Pods', undefined)).toBe(false);
  });
});

describe('mocRingSegments', () => {
  it('nests the read arc inside the started arc', () => {
    const { read, started } = mocRingSegments({ total: 10, read: 4, in_progress: 3 });
    expect(read).toBeCloseTo(0.4);
    expect(started).toBeCloseTo(0.7);
  });

  it('fills the ring when every note is read', () => {
    expect(mocRingSegments({ total: 10, read: 10, in_progress: 0 })).toEqual({ read: 1, started: 1 });
  });

  it('draws only the blue band when nothing is finished', () => {
    const { read, started } = mocRingSegments({ total: 10, read: 0, in_progress: 1 });
    expect(read).toBe(0);
    expect(started).toBeCloseTo(0.1);
  });

  it('floors a small nonzero count to a visible arc', () => {
    // A true 1/20 is a hairline that reads as an empty ring, contradicting the
    // "1/20" in the label beside it.
    const { read, started } = mocRingSegments({ total: 20, read: 1, in_progress: 0 });
    expect(read).toBeCloseTo(0.085);
    expect(started).toBeCloseTo(0.085);
  });

  it('never floors a zero count into a band', () => {
    expect(mocRingSegments({ total: 20, read: 0, in_progress: 0 })).toEqual({ read: 0, started: 0 });
  });

  it('shows a blue band past the green one when both counts are sub-floor', () => {
    const { read, started } = mocRingSegments({ total: 40, read: 1, in_progress: 1 });
    expect(read).toBeCloseTo(0.085);
    expect(started).toBeCloseTo(0.17);
    expect(started).toBeGreaterThan(read);
  });

  it('adds no blue band when nothing is in progress', () => {
    const { read, started } = mocRingSegments({ total: 40, read: 1, in_progress: 0 });
    expect(read).toEqual(started);
  });

  it('never exceeds a full turn', () => {
    const { read, started } = mocRingSegments({ total: 1, read: 0, in_progress: 1 });
    expect(started).toBe(1);
    expect(read).toBe(0);
  });

  it('leaves counts above the floor untouched', () => {
    const { read, started } = mocRingSegments({ total: 10, read: 4, in_progress: 3 });
    expect(read).toBeCloseTo(0.4);
    expect(started).toBeCloseTo(0.7);
  });

  it('returns an empty ring for a member-less hub', () => {
    expect(mocRingSegments({ total: 0, read: 0, in_progress: 0 })).toEqual({ read: 0, started: 0 });
  });

  it('returns an empty ring for a missing rollup', () => {
    expect(mocRingSegments(null)).toEqual({ read: 0, started: 0 });
    expect(mocRingSegments(undefined)).toEqual({ read: 0, started: 0 });
  });

  it('clamps counts that overrun the total', () => {
    expect(mocRingSegments({ total: 3, read: 9, in_progress: 9 })).toEqual({ read: 1, started: 1 });
    expect(mocRingSegments({ total: 4, read: 3, in_progress: 3 })).toEqual({ read: 0.75, started: 1 });
  });

  it('survives negative and non-finite counts', () => {
    expect(mocRingSegments({ total: 4, read: -2, in_progress: NaN })).toEqual({ read: 0, started: 0 });
    expect(mocRingSegments({ total: NaN, read: 1, in_progress: 1 })).toEqual({ read: 0, started: 0 });
  });
});
