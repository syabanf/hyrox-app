import { describe, expect, it } from 'vitest';
import { baseToPack, packLabel, packPriceToBase, packToBase } from '../erp';

// The client does the same conversion the server does, for previews: the form
// says "10 × CTN is 240 into stock" before anybody commits to it. If the two
// implementations disagree, the preview lies about what is about to happen.

describe('pack conversion', () => {
  it('multiplies and divides back to where it started', () => {
    expect(packToBase(10, 24)).toBe(240);
    expect(baseToPack(240, 24)).toBe(10);
  });

  it('does not round a part carton away', () => {
    // Fifty-one pieces really is 2.125 cartons; saying "2" loses three pieces
    // of stock every time somebody looks at the number.
    expect(baseToPack(51, 24)).toBe(2.125);
  });

  it('treats a missing factor as base units rather than dividing by zero', () => {
    expect(packToBase(5, 0)).toBe(5);
    expect(baseToPack(5, 0)).toBe(5);
  });

  it('turns a carton price into a unit price', () => {
    expect(packPriceToBase(396_000, 24)).toBe(16_500);
    expect(packPriceToBase(16_500, 1)).toBe(16_500);
  });

  it('labels a pack the way a shelf edge does', () => {
    expect(packLabel({ unitCode: 'CTN', factor: 24, isBase: false }, 'PCS')).toBe('CTN (24 PCS)');
    expect(packLabel({ unitCode: 'PCS', factor: 1, isBase: true }, 'PCS')).toBe('PCS');
  });
});
