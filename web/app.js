/* =========================================================================
   APP
   The whole UI is one Vue instance mounted on #app (see index.html) — the
   app is small enough that splitting it into components would cost more in
   ceremony than it saves. Data loading goes through the API client in
   api.js; all user-facing strings go through t()/plural() from i18n.js.
   ========================================================================= */
const { createApp } = Vue;

createApp({
  data() {
    return {
      // ---- Session / auth ----
      // Before login: cached locale from a previous session on this browser,
      // falling back to the browser's own language. After login: the user's
      // saved preference from the database (see applyServerLanguage).
      locale: getStoredLocale() || apiLocale,
      currentUser: null,
      authChecking: true, // true until the initial /auth/me check resolves
      authForm: { username: '', password: '' },
      authError: '',
      authLoading: false,

      // ---- Loaded data and the active tab ----
      tab: 'items',
      items: [],
      locations: [],
      loadingItems: true,
      loadError: '',

      // ---- Items tab filters ----
      query: '',
      activeLocation: null, // id of the selected filter chip, null = "All"

      // ---- Locations tab ----
      palette: ['#3D6B63','#D98E2B','#8E5A9E','#B5453A','#4A6FA5','#5C7A29','#A85C8C','#3F7D52','#5B4E9E'],
      newLocationName: '',
      newLocationColor: '#3D6B63',
      newLocationParentId: null,
      collapsed: {}, // location id -> true when its children are folded away

      // ---- Add/edit item sheet ----
      showItemSheet: false,
      editingId: null, // null = the sheet is creating a new item
      form: { name: '', locationId: null, notes: '', images: [] },
      formErrors: {},
      saving: false,
      uploadingImages: false,

      lightbox: { open: false, images: [], index: 0 },

      // ---- Pending delete confirmations (the object being deleted, or null) ----
      deleteTarget: null,
      deleteLocationTarget: null,

      // ---- Edit location sheet ----
      editLocationTarget: null, // location being edited, or null
      editLocationForm: { name: '', color: '#3D6B63' },
      editLocationSaving: false,

      // ---- Profile tab forms ----
      usernameForm: { username: '', currentPassword: '' },
      usernameFormError: '',
      usernameFormSaving: false,
      passwordForm: { currentPassword: '', newPassword: '', confirmPassword: '' },
      passwordFormError: '',
      passwordFormSaving: false,

      toastMessage: '',
      toastTimer: null,
    };
  },
  computed: {
    // Wordmark shown in the sidebar, topbar and auth card. Russian and
    // Spanish use a two-word pair joined by an accent-coloured middle dot
    // ("ГДЕ · ЧТО"); German and English are a single phrase with no dot.
    // Returned as parts so the template can colour the separator.
    brandWordmark() {
      const wordmarks = {
        ru: ['ГДЕ', 'ЧТО'],
        es: ['QUÉ', 'DÓNDE'],
        fr: ['OÙ', 'QUOI'],
        de: ['WO IST WAS'],
        en: ['WHEREABOUTS'],
      };
      return wordmarks[this.locale] || wordmarks.en;
    },
    filteredItems() {
      const q = this.query.trim().toLowerCase();
      const scope = this.activeLocation ? this.getDescendantIds(this.activeLocation) : null;
      return this.items.filter(it => {
        const matchesLocation = scope ? scope.has(it.locationId) : true;
        const matchesQuery = q
          ? (it.name.toLowerCase().includes(q) ||
             (it.notes || '').toLowerCase().includes(q) ||
             this.locationPathString(it.locationId).toLowerCase().includes(q))
          : true;
        return matchesLocation && matchesQuery;
      }).sort((a,b) => new Date(b.updatedAt) - new Date(a.updatedAt));
    },
    // Flat list of locations in depth-first order, with nesting depth.
    // Used in selects and filter chips — always fully expanded.
    locationOptionsFlat() {
      const result = [];
      const walk = (parentId, depth) => {
        this.locations
          .filter(l => (l.parentId || null) === parentId)
          .forEach(l => {
            result.push({ ...l, depth });
            walk(l.id, depth + 1);
          });
      };
      walk(null, 0);
      return result;
    },
    // Same as locationOptionsFlat, but capped to the user's configured nesting
    // depth for the filter chips on the Items tab (0 = no cap, show all).
    locationFilterChips() {
      const depth = this.currentUser ? this.currentUser.locationFilterDepth : 0;
      if (!depth) return this.locationOptionsFlat;
      return this.locationOptionsFlat.filter(loc => loc.depth < depth);
    },
    // Same as above but respects collapsed nodes — used for the tree on the Locations tab.
    locationTreeFlat() {
      const result = [];
      const walk = (parentId, depth) => {
        this.locations
          .filter(l => (l.parentId || null) === parentId)
          .forEach(l => {
            const hasChildren = this.locations.some(c => c.parentId === l.id);
            result.push({ loc: l, depth, hasChildren });
            if (hasChildren && !this.collapsed[l.id]) walk(l.id, depth + 1);
          });
      };
      walk(null, 0);
      return result;
    },
  },
  async mounted() {
    onUnauthorized(() => {
      // Session expired or was revoked (e.g. in another tab) — send the user
      // back to the login screen.
      this.currentUser = null;
    });

    try {
      this.currentUser = await authApi.me();
      this.applyServerLanguage(this.currentUser.language);
    } catch (e) {
      // Not authenticated — this is a normal state, just show the login screen.
    } finally {
      this.authChecking = false;
    }

    if (this.currentUser) {
      await this.loadData();
    }
  },
  watch: {
    // Any open overlay locks background scrolling — see syncBodyScrollLock.
    showItemSheet() { this.syncBodyScrollLock(); },
    deleteTarget() { this.syncBodyScrollLock(); },
    deleteLocationTarget() { this.syncBodyScrollLock(); },
    editLocationTarget() { this.syncBodyScrollLock(); },
    'lightbox.open'() { this.syncBodyScrollLock(); },
    // Keep everything that lives outside the Vue tree in sync with the
    // active locale: the API client's own error messages, <html lang> and
    // the document title.
    locale: {
      immediate: true,
      handler(val) {
        setApiLocale(val);
        document.documentElement.lang = val;
        document.title = translate(val, 'Whereabouts — item tracker');
      },
    },
  },
  methods: {
    // ---------- i18n and user settings ----------
    t(text, params) {
      return translate(this.locale, text, params);
    },
    plural(n, key) {
      return pluralize(this.locale, n, key);
    },
    // Applies a language reported by the backend. A supported value becomes
    // the active locale and is cached in localStorage; an empty/unsupported
    // one (e.g. a brand-new account) falls back to the local cache or the
    // browser's own language instead.
    applyServerLanguage(lang) {
      if (ALL_LOCALES.includes(lang)) {
        this.locale = lang;
        setStoredLocale(lang);
      } else {
        this.locale = getStoredLocale() || detectBrowserLocale();
      }
    },
    async setLanguage(lang) {
      if (lang === this.locale || !this.currentUser) return;
      const previous = this.locale;
      this.locale = lang;
      try {
        const updated = await userApi.updateLanguage(lang);
        this.currentUser = updated;
        this.applyServerLanguage(updated.language);
      } catch (e) {
        this.locale = previous;
        this.showToast(this.t('Error: ') + e.message);
      }
    },
    async setLocationFilterDepth(event) {
      if (!this.currentUser) return;
      const raw = event.target.value.trim();
      let value;
      if (raw === '') {
        value = 0; // empty field = show all levels
      } else {
        value = parseInt(raw, 10);
        if (isNaN(value) || value < 1) {
          // Not a valid depth (e.g. "0" or negative) — revert the field instead of saving it.
          event.target.value = this.currentUser.locationFilterDepth || '';
          return;
        }
      }
      if (value === this.currentUser.locationFilterDepth) return;
      const previous = this.currentUser.locationFilterDepth;
      this.currentUser.locationFilterDepth = value;
      try {
        const updated = await userApi.updateLocationFilterDepth(value);
        this.currentUser = updated;
      } catch (e) {
        this.currentUser.locationFilterDepth = previous;
        this.showToast(this.t('Error: ') + e.message);
      }
    },
    // ---------- Data loading ----------
    // Items and locations are always loaded together: almost every item view
    // needs its location's name and colour to render.
    async loadData() {
      this.loadingItems = true;
      this.loadError = '';
      try {
        const [items, locations] = await Promise.all([api.listItems(), api.listLocations()]);
        this.items = items || [];
        this.locations = locations || [];
      } catch (e) {
        this.loadError = e.message;
        this.showToast(e.message);
      } finally {
        this.loadingItems = false;
      }
    },

    // ---------- Location tree helpers ----------
    locationOf(id) {
      return this.locations.find(l => l.id === id) || { name: this.t('No location'), color: '#9a9481' };
    },
    hasChildLocations(id) {
      return this.locations.some(l => l.parentId === id);
    },
    // Set of the location's own id plus all descendant ids at any depth — handy for filtering.
    getDescendantIds(id) {
      const ids = new Set([id]);
      const collect = (parentId) => {
        this.locations.filter(l => l.parentId === parentId).forEach(child => {
          ids.add(child.id);
          collect(child.id);
        });
      };
      collect(id);
      return ids;
    },
    // Chain of locations from the root down to this one, inclusive.
    locationPath(id) {
      const path = [];
      let current = this.locations.find(l => l.id === id);
      let guard = 0;
      while (current && guard < 20) {
        path.unshift(current);
        current = current.parentId ? this.locations.find(l => l.id === current.parentId) : null;
        guard++;
      }
      return path;
    },
    locationPathString(id) {
      const path = this.locationPath(id);
      return path.length ? path.map(l => l.name).join(' › ') : this.t('No location');
    },
    // Is `id` an ancestor of the currently active filter chip? Used to
    // highlight the parent chain when a nested location is selected, since
    // the ›/›› depth markers alone don't say *whose* child a chip is.
    isAncestorOfActive(id) {
      if (!this.activeLocation || id === this.activeLocation) return false;
      return this.locationPath(this.activeLocation).some(l => l.id === id);
    },
    // Items directly in this location, not counting nested ones.
    directCountFor(locationId) {
      return this.items.filter(i => i.locationId === locationId).length;
    },
    // Items in this location and all its nested locations — what the UI displays.
    totalCountFor(locationId) {
      const ids = this.getDescendantIds(locationId);
      return this.items.filter(i => ids.has(i.locationId)).length;
    },
    toggleCollapse(id) {
      this.collapsed[id] = !this.collapsed[id];
    },
    // Pre-selects this location as the parent in the "new location" form
    // below the tree, then focuses the name field so it's ready to type into.
    startSubLocation(loc) {
      this.newLocationParentId = loc.id;
      this.$nextTick(() => this.$refs.newLocationInput?.focus());
    },

    // ---------- Presentation helpers ----------
    // Fallback avatar label for items without a photo.
    initials(name) {
      return (name || '?').trim().slice(0,2).toUpperCase();
    },
    // "5 minutes ago" / "2 days ago", in the largest unit that still fits.
    relativeTime(iso) {
      const diffMs = Date.now() - new Date(iso).getTime();
      const mins = Math.round(diffMs / 60000);
      if (mins < 1) return this.t('just now');
      if (mins < 60) return formatAgo(this.locale, mins, this.plural(mins, 'minutes'));
      const hours = Math.round(mins/60);
      if (hours < 24) return formatAgo(this.locale, hours, this.plural(hours, 'hours'));
      const days = Math.round(hours/24);
      if (days < 30) return formatAgo(this.locale, days, this.plural(days, 'days'));
      const months = Math.round(days/30);
      return formatAgo(this.locale, months, this.plural(months, 'months'));
    },
    showToast(msg) {
      this.toastMessage = msg;
      clearTimeout(this.toastTimer);
      this.toastTimer = setTimeout(() => this.toastMessage = '', 2200);
    },
    // On phones, an open bottom sheet/dialog shouldn't let the background scroll underneath it.
    syncBodyScrollLock() {
      const anyOpen = this.showItemSheet || !!this.deleteTarget || !!this.deleteLocationTarget || !!this.editLocationTarget || this.lightbox.open;
      document.body.classList.toggle('modal-open', anyOpen);
    },

    // ---------- Auth ----------
    async submitAuth() {
      this.authError = '';
      const username = this.authForm.username.trim();
      const password = this.authForm.password;

      if (!username || !password) {
        this.authError = this.t('Please fill in username and password');
        return;
      }

      this.authLoading = true;
      try {
        const user = await authApi.login({ username, password, language: this.locale });
        this.currentUser = user;
        this.applyServerLanguage(user.language);
        this.authForm = { username: '', password: '' };
        await this.loadData();
      } catch (e) {
        this.authError = e.message;
      } finally {
        this.authLoading = false;
      }
    },
    async logout() {
      try {
        await authApi.logout();
      } catch (e) {
        // Even if the logout request fails, clear the session locally.
      }
      this.currentUser = null;
      this.items = [];
      this.locations = [];
      // Back to the login screen — reuse the cached locale if we have one,
      // otherwise fall back to the browser's language again.
      this.locale = getStoredLocale() || detectBrowserLocale();
    },

    // ---------- Profile tab ----------
    // Opening the tab re-seeds both forms, so a half-filled edit (or a stale
    // error) from a previous visit never carries over.
    openProfile() {
      this.tab = 'profile';
      this.usernameForm = { username: this.currentUser.username, currentPassword: '' };
      this.usernameFormError = '';
      this.passwordForm = { currentPassword: '', newPassword: '', confirmPassword: '' };
      this.passwordFormError = '';
    },
    async submitUsernameChange() {
      this.usernameFormError = '';
      const username = this.usernameForm.username.trim();
      if (!username) {
        this.usernameFormError = this.t('Please enter a username');
        return;
      }
      if (username.length < 3) {
        this.usernameFormError = this.t('Username must be at least 3 characters');
        return;
      }
      if (!this.usernameForm.currentPassword) {
        this.usernameFormError = this.t('Enter your current password');
        return;
      }
      this.usernameFormSaving = true;
      try {
        const updated = await userApi.updateUsername({ username, currentPassword: this.usernameForm.currentPassword });
        this.currentUser = updated;
        this.usernameForm.currentPassword = '';
        this.showToast(this.t('Username updated'));
      } catch (e) {
        this.usernameFormError = e.message;
      } finally {
        this.usernameFormSaving = false;
      }
    },
    async submitPasswordChange() {
      this.passwordFormError = '';
      const { currentPassword, newPassword, confirmPassword } = this.passwordForm;
      if (!currentPassword || !newPassword) {
        this.passwordFormError = this.t('Please fill in both password fields');
        return;
      }
      if (newPassword !== confirmPassword) {
        this.passwordFormError = this.t('New passwords do not match');
        return;
      }
      this.passwordFormSaving = true;
      try {
        await userApi.changePassword({ currentPassword, newPassword });
        this.passwordForm = { currentPassword: '', newPassword: '', confirmPassword: '' };
        this.showToast(this.t('Password changed'));
      } catch (e) {
        this.passwordFormError = e.message;
      } finally {
        this.passwordFormSaving = false;
      }
    },

    // ---------- Add / edit item sheet ----------
    openAddItem() {
      this.editingId = null;
      this.form = { name: '', locationId: this.locations[0]?.id ?? null, notes: '', images: [] };
      this.formErrors = {};
      this.showItemSheet = true;
      this.$nextTick(() => this.$refs.nameInput?.focus());
    },
    // form.images is a copy, so cancelling the sheet leaves the original item
    // untouched. Existing photos come back from the API as "/files/…" links;
    // newly picked ones are data: URLs until saveItem sends them — the backend
    // tells the two apart and only writes the new ones to disk.
    openEditItem(it) {
      this.editingId = it.id;
      this.form = { name: it.name, locationId: it.locationId, notes: it.notes || '', images: [...(it.images || [])] };
      this.formErrors = {};
      this.showItemSheet = true;
    },

    // Photos are downscaled in the browser before upload purely to keep the
    // request small; the server re-checks and compresses again (see images.go),
    // so this is an optimization, not the size limit.
    async onFilesSelected(e) {
      const files = Array.from(e.target.files || []);
      e.target.value = ''; // let the same file be picked again after removing it
      if (!files.length) return;
      this.uploadingImages = true;
      try {
        for (const file of files) {
          if (!file.type.startsWith('image/')) continue;
          const dataUrl = await this.resizeImage(file, 1000);
          this.form.images.push(dataUrl);
        }
      } catch (err) {
        this.showToast(this.t('Failed to upload photo'));
      } finally {
        this.uploadingImages = false;
      }
    },
    // Draws the picked file onto a canvas at a capped size and re-encodes it
    // as a JPEG data URL.
    resizeImage(file, maxDim) {
      return new Promise((resolve, reject) => {
        const reader = new FileReader();
        reader.onerror = () => reject(reader.error);
        reader.onload = () => {
          const img = new Image();
          img.onerror = reject;
          img.onload = () => {
            let { width, height } = img;
            if (width > maxDim || height > maxDim) {
              const scale = maxDim / Math.max(width, height);
              width = Math.round(width * scale);
              height = Math.round(height * scale);
            }
            const canvas = document.createElement('canvas');
            canvas.width = width; canvas.height = height;
            canvas.getContext('2d').drawImage(img, 0, 0, width, height);
            resolve(canvas.toDataURL('image/jpeg', 0.82));
          };
          img.src = reader.result;
        };
        reader.readAsDataURL(file);
      });
    },
    removeFormImage(idx) {
      this.form.images.splice(idx, 1);
    },
    openLightbox(images, index) {
      this.lightbox = { open: true, images, index };
    },
    closeLightbox() {
      this.lightbox.open = false;
    },
    // dir is +1/-1; wraps around at both ends.
    lightboxStep(dir) {
      const len = this.lightbox.images.length;
      this.lightbox.index = (this.lightbox.index + dir + len) % len;
    },
    closeItemSheet() {
      this.showItemSheet = false;
    },
    validateForm() {
      const errs = {};
      if (!this.form.name.trim()) errs.name = this.t('Enter the item name');
      if (!this.form.locationId) errs.locationId = this.t('Choose a location');
      this.formErrors = errs;
      return Object.keys(errs).length === 0;
    },
    // The server answers with the saved record (photo data URLs already
    // replaced by their stored "/files/…" links), so the local list is
    // patched with the response rather than the form's own values.
    async saveItem() {
      if (!this.validateForm()) return;
      this.saving = true;
      try {
        const payload = { name: this.form.name.trim(), locationId: this.form.locationId, notes: this.form.notes.trim(), images: [...this.form.images] };
        if (this.editingId) {
          const updated = await api.updateItem(this.editingId, payload);
          const idx = this.items.findIndex(i => i.id === this.editingId);
          if (idx !== -1) this.items.splice(idx, 1, updated);
          this.showToast(this.t('Item updated'));
        } else {
          const created = await api.createItem(payload);
          this.items.unshift(created);
          this.showToast(this.t('Item added'));
        }
        this.showItemSheet = false;
      } catch (e) {
        this.showToast(this.t('Error: ') + e.message);
      } finally {
        this.saving = false;
      }
    },

    // ---------- Deleting items ----------
    askDelete(it) { this.deleteTarget = it; },
    async confirmDeleteItem() {
      const target = this.deleteTarget;
      this.deleteTarget = null;
      try {
        await api.deleteItem(target.id);
        this.items = this.items.filter(i => i.id !== target.id);
        this.showToast(this.t('Item deleted'));
      } catch (e) {
        this.showToast(this.t('Error: ') + e.message);
      }
    },

    // ---------- Locations tab ----------
    async createLocation() {
      const name = this.newLocationName.trim();
      if (!name) return;
      try {
        const created = await api.createLocation({ name, color: this.newLocationColor, parentId: this.newLocationParentId });
        this.locations.push(created);
        // Expand the parent so the location that was just added is visible.
        if (created.parentId) this.collapsed[created.parentId] = false;
        this.newLocationName = '';
        this.showToast(this.t('Location added'));
      } catch (e) {
        this.showToast(this.t('Error: ') + e.message);
      }
    },
    // Opens the edit sheet pre-filled with this location's current name/color.
    openEditLocation(loc) {
      this.editLocationTarget = loc;
      this.editLocationForm = { name: loc.name, color: loc.color };
    },
    closeEditLocation() {
      this.editLocationTarget = null;
    },
    async saveLocationEdit() {
      const name = this.editLocationForm.name.trim();
      if (!name) return;
      this.editLocationSaving = true;
      try {
        const updated = await api.updateLocation(this.editLocationTarget.id, { name, color: this.editLocationForm.color });
        const idx = this.locations.findIndex(l => l.id === updated.id);
        if (idx !== -1) this.locations.splice(idx, 1, updated);
        this.editLocationTarget = null;
        this.showToast(this.t('Location updated'));
      } catch (e) {
        this.showToast(this.t('Error: ') + e.message);
      } finally {
        this.editLocationSaving = false;
      }
    },
    askDeleteLocation(loc) { this.deleteLocationTarget = loc; },
    // A location is only removable once it's empty. The dialog already
    // disables the button in that case; this re-check guards the method
    // itself, and the backend rejects it too.
    async confirmDeleteLocation() {
      const target = this.deleteLocationTarget;
      if (this.hasChildLocations(target.id) || this.directCountFor(target.id) > 0) return;
      this.deleteLocationTarget = null;
      try {
        await api.deleteLocation(target.id);
        this.locations = this.locations.filter(l => l.id !== target.id);
        // Don't leave the filter pointing at a location that no longer exists.
        if (this.activeLocation === target.id) this.activeLocation = null;
        this.showToast(this.t('Location deleted'));
      } catch (e) {
        this.showToast(this.t('Error: ') + e.message);
      }
    },
  },
}).mount('#app');
