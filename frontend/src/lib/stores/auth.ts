// Auth Store — Svelte 5 reactive auth state with localStorage persistence
import { writable, derived } from 'svelte/store';
import { api } from '$lib/api/client';

interface User {
	user_id: string;
	email: string;
	full_name: string;
	tenant_id: string;
	tenant_name: string;
	default_role: string;
}

interface AuthState {
	user: User | null;
	token: string | null;
	refreshToken: string | null;
	loading: boolean;
	error: string | null;
}

function createAuthStore() {
	// Load from localStorage
	let initial: AuthState = {
		user: null,
		token: null,
		refreshToken: null,
		loading: false,
		error: null
	};

	if (typeof window !== 'undefined') {
		const savedUser = localStorage.getItem('rems_user');
		const savedToken = localStorage.getItem('rems_token');
		const savedRefresh = localStorage.getItem('rems_refresh_token');

		if (savedUser && savedToken) {
			try {
				initial = {
					user: JSON.parse(savedUser),
					token: savedToken,
					refreshToken: savedRefresh,
					loading: false,
					error: null
				};
			} catch {
				// corrupted data
			}
		}
	}

	const { subscribe, set, update } = writable<AuthState>(initial);

	return {
		subscribe,

		async login(email: string, password: string, deviceType = 'web') {
			update((s) => ({ ...s, loading: true, error: null }));

			const res = await api.post<{
				token?: string;
				refresh_token?: string;
				user?: User;
				requires_2fa?: boolean;
				pending_session_id?: string;
			}>('/auth/login', { email, password, device_type: deviceType });

			if (!res.success) {
				update((s) => ({ ...s, loading: false, error: res.message }));
				return { success: false, requires2FA: false, message: res.message };
			}

			if (res.data?.requires_2fa) {
				update((s) => ({ ...s, loading: false }));
				return {
					success: true,
					requires2FA: true,
					pendingSessionId: res.data.pending_session_id
				};
			}

			if (res.data?.token && res.data?.user) {
				api.setTokens(res.data.token, res.data.refresh_token || '');
				localStorage.setItem('rems_user', JSON.stringify(res.data.user));

				set({
					user: res.data.user,
					token: res.data.token,
					refreshToken: res.data.refresh_token || null,
					loading: false,
					error: null
				});
			}

			return { success: true, requires2FA: false };
		},

		async register(data: {
			email: string;
			user_name: string;
			password: string;
			password_confirm: string;
			full_name: string;
			phone?: string;
			tenant_name: string;
			tenant_id?: string;
		}) {
			update((s) => ({ ...s, loading: true, error: null }));

			const res = await api.post('/auth/register', {
				...data,
				tenant_id: data.tenant_id || '00000000-0000-0000-0000-000000000000'
			});

			if (!res.success) {
				update((s) => ({ ...s, loading: false, error: res.message }));
				return { success: false, message: res.message };
			}

			update((s) => ({ ...s, loading: false }));
			return { success: true };
		},

		async logout() {
			try {
				await api.post('/auth/logout');
			} catch {
				// fire-and-forget
			}

			api.clearTokens();
			set({
				user: null,
				token: null,
				refreshToken: null,
				loading: false,
				error: null
			});
		},

		clearError() {
			update((s) => ({ ...s, error: null }));
		}
	};
}

export const auth = createAuthStore();
export const isAuthenticated = derived(auth, ($a) => !!$a.token && !!$a.user);
export const currentUser = derived(auth, ($a) => $a.user);
