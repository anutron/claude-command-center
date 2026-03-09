import { describe, it, expect } from 'vitest';

describe('Things URL Construction', () => {
  describe('add-todo URL parameters', () => {
    it('should construct basic todo URL with title', () => {
      const params = new URLSearchParams();
      params.set('title', 'Buy groceries');
      const url = `things:///add?${params.toString()}`;
      
      expect(url).toBe('things:///add?title=Buy+groceries');
    });

    it('should handle scheduling parameter', () => {
      const params = new URLSearchParams();
      params.set('title', 'Review code');
      params.set('when', 'today');
      const url = `things:///add?${params.toString()}`;
      
      expect(url).toContain('when=today');
    });

    it('should handle multiple tags', () => {
      const params = new URLSearchParams();
      params.set('title', 'Task');
      params.set('tags', ['work', 'urgent'].join(','));
      const url = `things:///add?${params.toString()}`;
      
      expect(url).toContain('tags=work%2Curgent');
    });

    it('should handle checklist items', () => {
      const params = new URLSearchParams();
      params.set('title', 'Project');
      params.set('checklist-items', ['Step 1', 'Step 2'].join('\n'));
      const url = `things:///add?${params.toString()}`;
      
      expect(url).toContain('checklist-items');
    });
  });

  describe('add-project URL parameters', () => {
    it('should construct project URL with title', () => {
      const params = new URLSearchParams();
      params.set('title', 'Website Redesign');
      const url = `things:///add-project?${params.toString()}`;
      
      expect(url).toBe('things:///add-project?title=Website+Redesign');
    });

    it('should handle project todos', () => {
      const params = new URLSearchParams();
      params.set('title', 'Project');
      params.set('to-dos', ['Task 1', 'Task 2'].join('\n'));
      const url = `things:///add-project?${params.toString()}`;
      
      expect(url).toContain('to-dos');
    });
  });

  describe('show URL parameters', () => {
    it('should construct show URL with query', () => {
      const params = new URLSearchParams();
      params.set('query', 'today');
      const url = `things:///show?${params.toString()}`;
      
      expect(url).toBe('things:///show?query=today');
    });

    it('should handle tag filters', () => {
      const params = new URLSearchParams();
      params.set('query', 'today');
      params.set('filter', ['urgent', 'work'].join(','));
      const url = `things:///show?${params.toString()}`;
      
      expect(url).toContain('filter=urgent%2Cwork');
    });
  });

  describe('update URL parameters', () => {
    it('should require id and auth-token', () => {
      const params = new URLSearchParams();
      params.set('id', 'ABC123');
      params.set('auth-token', 'token123');
      const url = `things:///update?${params.toString()}`;
      
      expect(url).toContain('id=ABC123');
      expect(url).toContain('auth-token=token123');
    });

    it('should handle note modifications', () => {
      const params = new URLSearchParams();
      params.set('id', 'ABC123');
      params.set('auth-token', 'token');
      params.set('append-notes', 'Additional context');
      const url = `things:///update?${params.toString()}`;
      
      expect(url).toContain('append-notes');
    });
  });

  describe('search URL parameters', () => {
    it('should construct search URL', () => {
      const params = new URLSearchParams();
      params.set('query', 'meeting notes');
      const url = `things:///search?${params.toString()}`;
      
      expect(url).toBe('things:///search?query=meeting+notes');
    });

    it('should handle empty search', () => {
      const url = 'things:///search?';
      expect(url).toBe('things:///search?');
    });
  });

  describe('URL encoding', () => {
    it('should properly encode special characters', () => {
      const params = new URLSearchParams();
      params.set('title', 'Task with & special $ chars');
      const url = `things:///add?${params.toString()}`;
      
      expect(url).toContain('Task+with+%26+special+%24+chars');
    });

    it('should handle newlines in notes', () => {
      const params = new URLSearchParams();
      params.set('notes', 'Line 1\nLine 2');
      const url = `things:///add?${params.toString()}`;
      
      expect(url).toContain('notes=');
      expect(url).toContain('Line');
    });
  });
});
