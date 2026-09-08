# Story Screens

Screens for displaying system-generated community story cards.

## Purpose

Stories are celebratory, auto-generated highlights surfaced in the community feed — completed loans, giveaways, new members, and similar milestones. This directory contains the full-screen story viewer.

## Key Files

- **`story_content_view.dart`** — the story body: background media, title, description, participant avatars, and undo action if available.

## When to add here vs. elsewhere

Story display belongs here. Story-related widgets that could be reused in other contexts (e.g., participant avatar stacks) live in `widgets/shared/` or `widgets/content/`. Feed items that link to stories are rendered by the feed in `screens/home/`.
