#!/usr/bin/env bun

import { DiskDB } from '../src/db';

const db = new DiskDB();
const sets = db.listSelectionSets();

console.log('Current Selection Sets:\n');
for (const set of sets) {
  const stats = db.getSelectionSetStats(set.name);
  console.log(`${set.name}:`);
  console.log(`  Type: ${set.criteria_type}`);
  console.log(`  Description: ${set.description || 'N/A'}`);
  console.log(`  Files: ${stats.count}`);
  console.log(`  Total Size: ${(stats.totalSize / 1024 / 1024).toFixed(2)} MB`);
  console.log(`  Created: ${new Date(set.created_at! * 1000).toLocaleString()}`);
  if (set.criteria_json) {
    console.log(`  Criteria: ${set.criteria_json}`);
  }
  console.log();
}