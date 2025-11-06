import '@testing-library/jest-dom'
import { expect } from 'vitest'
// Try to import matchers in a way that works across versions/exports
import * as matchersModule from '@testing-library/jest-dom/matchers'
const matchers = matchersModule.default || matchersModule

// extends vitest's expect with jest-dom matchers
expect.extend(matchers)
