# Metadata Log

## Append Only Log

This could live in the metadata log as a new operation type, consistent with the existing append-only architecture. Hatcheck already uses an append-only log with indexes rebuilt on startup for its content metadata. 

## Operations

## Indexes

## Queries

A RoleIndex following the same pattern as your existing indexes — built from the log on startup, queryable by principal or by role. Two useful queries: