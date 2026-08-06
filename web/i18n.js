/* =========================================================================
   I18N — English, Russian, German, Spanish and French
   Login screen: locale is the last one cached in localStorage (see
   getStoredLocale below), falling back to the browser's language.
   After login: locale comes from the user's language preference stored in
   the database (see PublicUser.language in auth.go) — the database is the
   source of truth across devices; localStorage is just a local cache so the
   right language shows up before the backend has answered.
   ========================================================================= */

// English is handled by the fallback path (see translate below), so it isn't
// listed here — this is the set of locales that have their own tables.
// Keep in sync with supportedLanguages in auth.go.
const SUPPORTED_LOCALES = ['ru', 'de', 'es', 'fr'];

// Every language code an account can actually be saved with, including
// English. Use this (not SUPPORTED_LOCALES) when validating a language value
// that came from the server or localStorage — SUPPORTED_LOCALES on its own
// would treat a legitimate 'en' as unrecognized.
const ALL_LOCALES = ['en', ...SUPPORTED_LOCALES];

function detectBrowserLocale() {
  const langs = (navigator.languages && navigator.languages.length) ? navigator.languages : [navigator.language || ''];
  for (const l of langs) {
    const lower = (l || '').toLowerCase();
    const match = SUPPORTED_LOCALES.find(loc => lower.startsWith(loc));
    if (match) return match;
  }
  return 'en';
}

// Local cache of the backend-confirmed language, keyed per browser. Written
// every time the server hands back a user with a language on it (login,
// explicit change); read only to guess the locale before the backend has
// answered (e.g. the login screen, or while /auth/me is in flight).
const LOCALE_STORAGE_KEY = 'whereabouts.locale';
function getStoredLocale() {
  try {
    const val = localStorage.getItem(LOCALE_STORAGE_KEY);
    return ALL_LOCALES.includes(val) ? val : null;
  } catch (e) {
    return null; // private browsing / storage disabled
  }
}
function setStoredLocale(locale) {
  try { localStorage.setItem(LOCALE_STORAGE_KEY, locale); } catch (e) { /* ignore */ }
}

// English is the default language, so English strings are written directly
// at each call site (e.g. t('Add item')) and used as-is when locale is 'en'.
// These tables only need to hold the translation for each string.
const TRANSLATIONS = {
  ru: {
    'Whereabouts — item tracker': 'Где что — учёт вещей',
    'Checking session…': 'Проверяем сессию…',
    'Track where your things are': 'Учёт местонахождения вещей',
    'Username': 'Имя пользователя',
    'Password': 'Пароль',
    'At least 3 characters': 'Минимум 3 символа',
    'At least 4 characters': 'Минимум 4 символа',
    'Log in': 'Войти',
    'Logging in…': 'Проверяем…',
    'Please fill in username and password': 'Заполните имя пользователя и пароль',
    'Please enter a username': 'Введите имя пользователя',
    'Username must be at least 3 characters': 'Имя пользователя должно быть не короче 3 символов',

    'Search items…': 'Найти вещь…',
    'Profile': 'Профиль',
    'Log out': 'Выйти',
    'Interface language': 'Язык интерфейса',
    'Location nesting depth in filters': 'Глубина вложенности мест в фильтрах',
    'How many levels of nested locations to show as filter chips on the Items tab. Leave empty to show all levels.': 'Сколько уровней вложенности мест показывать в виде фильтров на вкладке «Вещи». Оставьте поле пустым, чтобы показывать все уровни.',
    'Change username': 'Смена имени пользователя',
    'Change password': 'Смена пароля',
    'Current password': 'Текущий пароль',
    'New password': 'Новый пароль',
    'Confirm new password': 'Повторите новый пароль',
    'Enter your current password': 'Введите текущий пароль',
    'Please fill in both password fields': 'Заполните оба поля пароля',
    'New passwords do not match': 'Новые пароли не совпадают',
    'Username updated': 'Имя пользователя обновлено',
    'Password changed': 'Пароль изменён',

    'All': 'Все',
    'reset filter ✕': 'сбросить фильтр ✕',
    'Backend unavailable': 'Бэкенд недоступен',
    'Try again': 'Повторить попытку',
    'Nothing found': 'Ничего не найдено',
    'Nothing here yet': 'Пока пусто',
    'Try changing the search query or location filter.': 'Попробуйте изменить запрос или фильтр по месту.',
    'Add your first item and remember where you put it.': 'Добавьте первую вещь и запомните, куда вы её положили.',
    'Add item': 'Добавить вещь',
    'Edit': 'Изменить',
    'Delete': 'Удалить',
    'Updated': 'Обновлено',

    'Storage locations': 'Места хранения',
    'Collapse/expand': 'Свернуть/развернуть',
    'Add nested location': 'Добавить вложенное место',
    'Delete location': 'Удалить место',
    'New location': 'Новое место',
    'Inside:': 'Внутри:',
    'remove ✕': 'убрать ✕',
    'Parent location': 'Родительское место',
    '— No parent (top level) —': '— Без родителя (верхний уровень) —',
    'e.g. Shelf 2': 'Например: Полка 2',
    'Add': 'Добавить',

    'Items': 'Вещи',
    'Locations': 'Места',

    'Edit item': 'Изменить вещь',
    'New item': 'Новая вещь',
    'Item name': 'Название вещи',
    'e.g. Passport': 'Например: Паспорт',
    'Location': 'Место хранения',
    'Select a location': 'Выберите место',
    'Note (optional)': 'Заметка (необязательно)',
    'e.g. top drawer, under documents': 'Например: в верхнем ящике, под документами',
    'Photos (optional)': 'Фотографии (необязательно)',
    'Remove photo': 'Удалить фото',
    'Cancel': 'Отмена',
    'Save': 'Сохранить',
    'Saving…': 'Сохранение…',

    'Enter the item name': 'Введите название вещи',
    'Choose a location': 'Выберите место хранения',
    'Failed to upload photo': 'Не удалось загрузить фото',

    'Delete "{name}"?': 'Удалить «{name}»?',
    "This item's location record will be permanently deleted.": 'Запись о местонахождении этой вещи будет удалена без возможности восстановления.',
    'Delete location "{name}"?': 'Удалить место «{name}»?',
    'It contains nested locations. Delete or move them first.': 'Внутри есть вложенные места. Сначала удалите или перенесите их.',
    'This location holds {n} {word}. Move them to another location first.': 'В этом месте хранится {n} {word}. Сначала перенесите их в другое место.',
    'This storage location will be deleted.': 'Это место хранения будет удалено.',

    'Item updated': 'Запись обновлена',
    'Item added': 'Вещь добавлена',
    'Item deleted': 'Вещь удалена',
    'Location added': 'Место добавлено',
    'Location deleted': 'Место удалено',
    'Error: ': 'Ошибка: ',

    'Data is stored on the server (Go + MariaDB)': 'Данные хранятся на сервере (Go + MariaDB)',
    'No location': 'Без места',

    'just now': 'только что',

    'Request error ({status})': 'Ошибка запроса ({status})',
    'Backend unavailable. Check that the Go server is running.': 'Бэкенд недоступен. Проверьте, что сервер на Go запущен.',
  },

  de: {
    'Whereabouts — item tracker': 'Whereabouts — Verzeichnis für Gegenstände',
    'Checking session…': 'Sitzung wird geprüft…',
    'Track where your things are': 'Behalten Sie den Überblick, wo Ihre Sachen sind',
    'Username': 'Benutzername',
    'Password': 'Passwort',
    'At least 3 characters': 'Mindestens 3 Zeichen',
    'At least 4 characters': 'Mindestens 4 Zeichen',
    'Log in': 'Anmelden',
    'Logging in…': 'Anmeldung läuft…',
    'Please fill in username and password': 'Bitte Benutzername und Passwort eingeben',
    'Please enter a username': 'Bitte einen Benutzernamen eingeben',
    'Username must be at least 3 characters': 'Der Benutzername muss mindestens 3 Zeichen lang sein',

    'Search items…': 'Gegenstände suchen…',
    'Profile': 'Profil',
    'Log out': 'Abmelden',
    'Interface language': 'Sprache der Oberfläche',
    'Location nesting depth in filters': 'Verschachtelungstiefe der Orte in Filtern',
    'How many levels of nested locations to show as filter chips on the Items tab. Leave empty to show all levels.': 'Wie viele Verschachtelungsebenen von Orten als Filter im Tab „Gegenstände" angezeigt werden. Leer lassen, um alle Ebenen anzuzeigen.',
    'Change username': 'Benutzername ändern',
    'Change password': 'Passwort ändern',
    'Current password': 'Aktuelles Passwort',
    'New password': 'Neues Passwort',
    'Confirm new password': 'Neues Passwort bestätigen',
    'Enter your current password': 'Geben Sie Ihr aktuelles Passwort ein',
    'Please fill in both password fields': 'Bitte beide Passwortfelder ausfüllen',
    'New passwords do not match': 'Die neuen Passwörter stimmen nicht überein',
    'Username updated': 'Benutzername aktualisiert',
    'Password changed': 'Passwort geändert',

    'All': 'Alle',
    'reset filter ✕': 'Filter zurücksetzen ✕',
    'Backend unavailable': 'Backend nicht erreichbar',
    'Try again': 'Erneut versuchen',
    'Nothing found': 'Nichts gefunden',
    'Nothing here yet': 'Hier ist noch nichts',
    'Try changing the search query or location filter.': 'Ändern Sie die Suchanfrage oder den Ortsfilter.',
    'Add your first item and remember where you put it.': 'Fügen Sie Ihren ersten Gegenstand hinzu und merken Sie sich, wohin Sie ihn gelegt haben.',
    'Add item': 'Gegenstand hinzufügen',
    'Edit': 'Bearbeiten',
    'Delete': 'Löschen',
    'Updated': 'Aktualisiert',

    'Storage locations': 'Aufbewahrungsorte',
    'Collapse/expand': 'Ein-/ausklappen',
    'Add nested location': 'Unterort hinzufügen',
    'Delete location': 'Ort löschen',
    'New location': 'Neuer Ort',
    'Inside:': 'Innerhalb von:',
    'remove ✕': 'entfernen ✕',
    'Parent location': 'Übergeordneter Ort',
    '— No parent (top level) —': '— Kein übergeordneter Ort (oberste Ebene) —',
    'e.g. Shelf 2': 'z. B. Regal 2',
    'Add': 'Hinzufügen',

    'Items': 'Gegenstände',
    'Locations': 'Orte',

    'Edit item': 'Gegenstand bearbeiten',
    'New item': 'Neuer Gegenstand',
    'Item name': 'Name des Gegenstands',
    'e.g. Passport': 'z. B. Reisepass',
    'Location': 'Ort',
    'Select a location': 'Ort auswählen',
    'Note (optional)': 'Notiz (optional)',
    'e.g. top drawer, under documents': 'z. B. oberste Schublade, unter den Dokumenten',
    'Photos (optional)': 'Fotos (optional)',
    'Remove photo': 'Foto entfernen',
    'Cancel': 'Abbrechen',
    'Save': 'Speichern',
    'Saving…': 'Wird gespeichert…',

    'Enter the item name': 'Geben Sie den Namen des Gegenstands ein',
    'Choose a location': 'Wählen Sie einen Ort',
    'Failed to upload photo': 'Foto konnte nicht hochgeladen werden',

    'Delete "{name}"?': '„{name}“ löschen?',
    "This item's location record will be permanently deleted.": 'Der Ort-Eintrag dieses Gegenstands wird endgültig gelöscht.',
    'Delete location "{name}"?': 'Ort „{name}“ löschen?',
    'It contains nested locations. Delete or move them first.': 'Er enthält Unterorte. Löschen oder verschieben Sie diese zuerst.',
    'This location holds {n} {word}. Move them to another location first.': 'An diesem Ort befinden sich {n} {word}. Verschieben Sie diese zuerst an einen anderen Ort.',
    'This storage location will be deleted.': 'Dieser Aufbewahrungsort wird gelöscht.',

    'Item updated': 'Gegenstand aktualisiert',
    'Item added': 'Gegenstand hinzugefügt',
    'Item deleted': 'Gegenstand gelöscht',
    'Location added': 'Ort hinzugefügt',
    'Location deleted': 'Ort gelöscht',
    'Error: ': 'Fehler: ',

    'Data is stored on the server (Go + MariaDB)': 'Daten werden auf dem Server gespeichert (Go + MariaDB)',
    'No location': 'Kein Ort',

    'just now': 'gerade eben',

    'Request error ({status})': 'Anfragefehler ({status})',
    'Backend unavailable. Check that the Go server is running.': 'Backend nicht erreichbar. Prüfen Sie, ob der Go-Server läuft.',
  },

  es: {
    'Whereabouts — item tracker': 'Whereabouts — organizador de objetos',
    'Checking session…': 'Comprobando la sesión…',
    'Track where your things are': 'Controla dónde están tus cosas',
    'Username': 'Nombre de usuario',
    'Password': 'Contraseña',
    'At least 3 characters': 'Al menos 3 caracteres',
    'At least 4 characters': 'Al menos 4 caracteres',
    'Log in': 'Iniciar sesión',
    'Logging in…': 'Iniciando sesión…',
    'Please fill in username and password': 'Completa el nombre de usuario y la contraseña',
    'Please enter a username': 'Introduce un nombre de usuario',
    'Username must be at least 3 characters': 'El nombre de usuario debe tener al menos 3 caracteres',

    'Search items…': 'Buscar objetos…',
    'Profile': 'Perfil',
    'Log out': 'Cerrar sesión',
    'Interface language': 'Idioma de la interfaz',
    'Location nesting depth in filters': 'Profundidad de anidación de ubicaciones en filtros',
    'How many levels of nested locations to show as filter chips on the Items tab. Leave empty to show all levels.': 'Cuántos niveles de ubicaciones anidadas mostrar como filtros en la pestaña "Objetos". Déjelo vacío para mostrar todos los niveles.',
    'Change username': 'Cambiar nombre de usuario',
    'Change password': 'Cambiar contraseña',
    'Current password': 'Contraseña actual',
    'New password': 'Nueva contraseña',
    'Confirm new password': 'Confirmar nueva contraseña',
    'Enter your current password': 'Introduce tu contraseña actual',
    'Please fill in both password fields': 'Completa ambos campos de contraseña',
    'New passwords do not match': 'Las nuevas contraseñas no coinciden',
    'Username updated': 'Nombre de usuario actualizado',
    'Password changed': 'Contraseña cambiada',

    'All': 'Todos',
    'reset filter ✕': 'quitar filtro ✕',
    'Backend unavailable': 'Backend no disponible',
    'Try again': 'Intentar de nuevo',
    'Nothing found': 'No se encontró nada',
    'Nothing here yet': 'Aún no hay nada aquí',
    'Try changing the search query or location filter.': 'Prueba a cambiar la búsqueda o el filtro de ubicación.',
    'Add your first item and remember where you put it.': 'Añade tu primer objeto y recuerda dónde lo guardaste.',
    'Add item': 'Añadir objeto',
    'Edit': 'Editar',
    'Delete': 'Eliminar',
    'Updated': 'Actualizado',

    'Storage locations': 'Lugares de almacenamiento',
    'Collapse/expand': 'Contraer/expandir',
    'Add nested location': 'Añadir ubicación anidada',
    'Delete location': 'Eliminar ubicación',
    'New location': 'Nueva ubicación',
    'Inside:': 'Dentro de:',
    'remove ✕': 'quitar ✕',
    'Parent location': 'Ubicación superior',
    '— No parent (top level) —': '— Sin ubicación superior (nivel raíz) —',
    'e.g. Shelf 2': 'p. ej. Estante 2',
    'Add': 'Añadir',

    'Items': 'Objetos',
    'Locations': 'Ubicaciones',

    'Edit item': 'Editar objeto',
    'New item': 'Nuevo objeto',
    'Item name': 'Nombre del objeto',
    'e.g. Passport': 'p. ej. Pasaporte',
    'Location': 'Ubicación',
    'Select a location': 'Selecciona una ubicación',
    'Note (optional)': 'Nota (opcional)',
    'e.g. top drawer, under documents': 'p. ej. cajón superior, debajo de los documentos',
    'Photos (optional)': 'Fotos (opcional)',
    'Remove photo': 'Quitar foto',
    'Cancel': 'Cancelar',
    'Save': 'Guardar',
    'Saving…': 'Guardando…',

    'Enter the item name': 'Introduce el nombre del objeto',
    'Choose a location': 'Elige una ubicación',
    'Failed to upload photo': 'No se pudo subir la foto',

    'Delete "{name}"?': '¿Eliminar «{name}»?',
    "This item's location record will be permanently deleted.": 'El registro de ubicación de este objeto se eliminará de forma permanente.',
    'Delete location "{name}"?': '¿Eliminar la ubicación «{name}»?',
    'It contains nested locations. Delete or move them first.': 'Contiene ubicaciones anidadas. Elimínalas o muévelas primero.',
    'This location holds {n} {word}. Move them to another location first.': 'Esta ubicación contiene {n} {word}. Muévelos a otra ubicación primero.',
    'This storage location will be deleted.': 'Este lugar de almacenamiento se eliminará.',

    'Item updated': 'Objeto actualizado',
    'Item added': 'Objeto añadido',
    'Item deleted': 'Objeto eliminado',
    'Location added': 'Ubicación añadida',
    'Location deleted': 'Ubicación eliminada',
    'Error: ': 'Error: ',

    'Data is stored on the server (Go + MariaDB)': 'Los datos se guardan en el servidor (Go + MariaDB)',
    'No location': 'Sin ubicación',

    'just now': 'justo ahora',

    'Request error ({status})': 'Error de solicitud ({status})',
    'Backend unavailable. Check that the Go server is running.': 'Backend no disponible. Comprueba que el servidor Go esté en ejecución.',
  },

  fr: {
    'Whereabouts — item tracker': 'Où quoi — suivi de vos affaires',
    'Checking session…': 'Vérification de la session…',
    'Track where your things are': 'Gardez la trace de vos affaires',
    'Username': "Nom d'utilisateur",
    'Password': 'Mot de passe',
    'At least 3 characters': 'Au moins 3 caractères',
    'At least 4 characters': 'Au moins 4 caractères',
    'Log in': 'Se connecter',
    'Logging in…': 'Connexion…',
    'Please fill in username and password': "Renseignez le nom d'utilisateur et le mot de passe",
    'Please enter a username': "Saisissez un nom d'utilisateur",
    'Username must be at least 3 characters': "Le nom d'utilisateur doit comporter au moins 3 caractères",

    'Search items…': 'Rechercher…',
    'Profile': 'Profil',
    'Log out': 'Se déconnecter',
    'Interface language': "Langue de l'interface",
    'Location nesting depth in filters': "Profondeur d'imbrication des emplacements dans les filtres",
    'How many levels of nested locations to show as filter chips on the Items tab. Leave empty to show all levels.': "Combien de niveaux d'emplacements imbriqués afficher comme filtres dans l'onglet « Objets ». Laissez vide pour afficher tous les niveaux.",
    'Change username': "Changer le nom d'utilisateur",
    'Change password': 'Changer le mot de passe',
    'Current password': 'Mot de passe actuel',
    'New password': 'Nouveau mot de passe',
    'Confirm new password': 'Confirmez le nouveau mot de passe',
    'Enter your current password': 'Saisissez votre mot de passe actuel',
    'Please fill in both password fields': 'Remplissez les deux champs de mot de passe',
    'New passwords do not match': 'Les nouveaux mots de passe ne correspondent pas',
    'Username updated': "Nom d'utilisateur mis à jour",
    'Password changed': 'Mot de passe modifié',

    'All': 'Tous',
    'reset filter ✕': 'réinitialiser le filtre ✕',
    'Backend unavailable': 'Backend indisponible',
    'Try again': 'Réessayer',
    'Nothing found': 'Aucun résultat',
    'Nothing here yet': 'Rien pour le moment',
    'Try changing the search query or location filter.': "Essayez de modifier la recherche ou le filtre d'emplacement.",
    'Add your first item and remember where you put it.': "Ajoutez votre premier objet et souvenez-vous où vous l'avez rangé.",
    'Add item': 'Ajouter un objet',
    'Edit': 'Modifier',
    'Delete': 'Supprimer',
    'Updated': 'Mis à jour',

    'Storage locations': 'Emplacements de rangement',
    'Collapse/expand': 'Réduire/développer',
    'Add nested location': 'Ajouter un emplacement imbriqué',
    'Delete location': "Supprimer l'emplacement",
    'New location': 'Nouvel emplacement',
    'Inside:': 'Dans :',
    'remove ✕': 'retirer ✕',
    'Parent location': 'Emplacement parent',
    '— No parent (top level) —': '— Sans parent (niveau supérieur) —',
    'e.g. Shelf 2': 'Ex. : Étagère 2',
    'Add': 'Ajouter',

    'Items': 'Objets',
    'Locations': 'Emplacements',

    'Edit item': "Modifier l'objet",
    'New item': 'Nouvel objet',
    'Item name': "Nom de l'objet",
    'e.g. Passport': 'Ex. : Passeport',
    'Location': 'Emplacement',
    'Select a location': 'Choisissez un emplacement',
    'Note (optional)': 'Note (facultatif)',
    'e.g. top drawer, under documents': 'Ex. : tiroir du haut, sous les documents',
    'Photos (optional)': 'Photos (facultatif)',
    'Remove photo': 'Supprimer la photo',
    'Cancel': 'Annuler',
    'Save': 'Enregistrer',
    'Saving…': 'Enregistrement…',

    'Enter the item name': "Saisissez le nom de l'objet",
    'Choose a location': 'Choisissez un emplacement',
    'Failed to upload photo': 'Échec du téléversement de la photo',

    'Delete "{name}"?': 'Supprimer « {name} » ?',
    "This item's location record will be permanently deleted.": "L'enregistrement de l'emplacement de cet objet sera définitivement supprimé.",
    'Delete location "{name}"?': "Supprimer l'emplacement « {name} » ?",
    'It contains nested locations. Delete or move them first.': "Il contient des emplacements imbriqués. Supprimez-les ou déplacez-les d'abord.",
    'This location holds {n} {word}. Move them to another location first.': "Cet emplacement contient {n} {word}. Déplacez-les d'abord vers un autre emplacement.",
    'This storage location will be deleted.': 'Cet emplacement de rangement sera supprimé.',

    'Item updated': 'Objet mis à jour',
    'Item added': 'Objet ajouté',
    'Item deleted': 'Objet supprimé',
    'Location added': 'Emplacement ajouté',
    'Location deleted': 'Emplacement supprimé',
    'Error: ': 'Erreur : ',

    'Data is stored on the server (Go + MariaDB)': 'Les données sont stockées sur le serveur (Go + MariaDB)',
    'No location': 'Sans emplacement',

    'just now': "à l'instant",

    'Request error ({status})': 'Erreur de requête ({status})',
    'Backend unavailable. Check that the Go server is running.': 'Backend indisponible. Vérifiez que le serveur Go est démarré.',
  },
};

const PLURALS = {
  en: {
    items: ['item', 'items'],
    records: ['record', 'records'],
    minutes: ['minute', 'minutes'],
    hours: ['hour', 'hours'],
    days: ['day', 'days'],
    months: ['month', 'months'],
  },
  ru: {
    items: ['вещь', 'вещи', 'вещей'],
    records: ['запись', 'записи', 'записей'],
    minutes: ['минуту', 'минуты', 'минут'],
    hours: ['час', 'часа', 'часов'],
    days: ['день', 'дня', 'дней'],
    months: ['месяц', 'месяца', 'месяцев'],
  },
  de: {
    items: ['Gegenstand', 'Gegenstände'],
    records: ['Eintrag', 'Einträge'],
    minutes: ['Minute', 'Minuten'],
    hours: ['Stunde', 'Stunden'],
    days: ['Tag', 'Tage'],
    months: ['Monat', 'Monate'],
  },
  es: {
    items: ['objeto', 'objetos'],
    records: ['registro', 'registros'],
    minutes: ['minuto', 'minutos'],
    hours: ['hora', 'horas'],
    days: ['día', 'días'],
    months: ['mes', 'meses'],
  },
  fr: {
    items: ['objet', 'objets'],
    records: ['enregistrement', 'enregistrements'],
    minutes: ['minute', 'minutes'],
    hours: ['heure', 'heures'],
    days: ['jour', 'jours'],
    months: ['mois', 'mois'], // "mois" is invariable
  },
};

// Russian needs three plural forms (1 / 2-4 / 5+, with exceptions for 11-14).
// French takes the singular for 0 as well as 1 ("0 objet"), unlike English,
// German and Spanish, which only single out 1.
function pluralize(locale, n, key) {
  const forms = PLURALS[locale][key];
  if (locale === 'ru') {
    const mod10 = n % 10, mod100 = n % 100;
    if (mod10 === 1 && mod100 !== 11) return forms[0];
    if ([2, 3, 4].includes(mod10) && ![12, 13, 14].includes(mod100)) return forms[1];
    return forms[2];
  }
  if (locale === 'fr') return n <= 1 ? forms[0] : forms[1];
  return n === 1 ? forms[0] : forms[1];
}

// `text` is the English source string — used directly as both the lookup
// key and the English fallback, so English never needs its own table.
function translate(locale, text, params) {
  const table = TRANSLATIONS[locale];
  let s = table ? (table[text] ?? text) : text;
  if (params) {
    for (const k of Object.keys(params)) {
      s = s.replace(new RegExp('\\{' + k + '\\}', 'g'), params[k]);
    }
  }
  return s;
}

// Relative-time phrases ("5 minutes ago") don't share word order across
// languages — Russian puts the "ago" word after the number like English
// does, but German, Spanish and French put their equivalent before it
// ("vor 5 Minuten", "hace 5 minutos", "il y a 5 minutes") — so this is a
// template per locale rather than a single translatable suffix word.
const AGO_TEMPLATES = {
  en: (n, unit) => `${n} ${unit} ago`,
  ru: (n, unit) => `${n} ${unit} назад`,
  de: (n, unit) => `vor ${n} ${unit}`,
  es: (n, unit) => `hace ${n} ${unit}`,
  fr: (n, unit) => `il y a ${n} ${unit}`,
};
function formatAgo(locale, n, unit) {
  return (AGO_TEMPLATES[locale] || AGO_TEMPLATES.en)(n, unit);
}

// Locale used by the API client to localize its own error messages (network
// unavailable, unexpected status, etc). The Vue app keeps this in sync with
// its own `locale` state via setApiLocale() — see app.js.
let apiLocale = detectBrowserLocale();
function setApiLocale(locale) { apiLocale = locale; }

// The backend always replies in English (see handlers.go/auth.go) — a full
// server-side i18n layer would be overkill for a dozen-and-a-half strings,
// so known messages are translated here by exact match, per locale.
const SERVER_ERRORS = {
  ru: {
    'Authentication required': 'Необходима авторизация',
    'Invalid request body': 'Некорректное тело запроса',
    'Please enter a username': 'Укажите имя пользователя',
    'Username must be at least 3 characters': 'Имя пользователя должно быть не короче 3 символов',
    'Password must be at least 4 characters': 'Пароль должен быть не короче 4 символов',
    'Failed to process password': 'Не удалось обработать пароль',
    'A user with this username is already registered': 'Пользователь с таким именем уже зарегистрирован',
    'Failed to create user': 'Не удалось создать пользователя',
    'User created, but failed to start a session': 'Пользователь создан, но не удалось начать сессию',
    'Incorrect username or password': 'Неверное имя пользователя или пароль',
    'Failed to start a session': 'Не удалось начать сессию',
    'Not authenticated': 'Не авторизован',
    'Unsupported language': 'Недопустимый язык',
    'Failed to save language preference': 'Не удалось сохранить язык',
    'Failed to verify current password': 'Не удалось проверить текущий пароль',
    'Incorrect current password': 'Неверный текущий пароль',
    'Failed to update username': 'Не удалось обновить имя пользователя',
    'Failed to update password': 'Не удалось обновить пароль',
    'Failed to load items': 'Не удалось получить вещи',
    'Failed to read item': 'Не удалось прочитать вещь',
    'Failed to load photos': 'Не удалось получить фотографии',
    'Please check the form fields': 'Проверьте поля формы',
    'Enter the item name': 'Введите название вещи',
    'Choose a location': 'Выберите место хранения',
    'Failed to verify location': 'Не удалось проверить место',
    'The specified location was not found': 'Указанное место хранения не найдено',
    'Failed to save item': 'Не удалось сохранить вещь',
    'Failed to save photos': 'Не удалось сохранить фотографии',
    'Item saved, but failed to read it back': 'Вещь сохранена, но не удалось её прочитать',
    'Record not found': 'Запись не найдена',
    'Failed to update item': 'Не удалось обновить вещь',
    'Failed to update photos': 'Не удалось обновить фотографии',
    'Item updated, but failed to read it back': 'Вещь обновлена, но не удалось её прочитать',
    'Failed to delete item': 'Не удалось удалить вещь',
    'Failed to load locations': 'Не удалось получить места',
    'Failed to read location': 'Не удалось прочитать место',
    'Please enter a location name': 'Введите название места',
    'Failed to verify parent location': 'Не удалось проверить родительское место',
    'Parent location not found': 'Родительское место не найдено',
    'Failed to save location': 'Не удалось сохранить место',
    'Failed to check nested locations': 'Не удалось проверить вложенные места',
    'This location has nested locations — delete or move them first': 'У этого места есть вложенные места — сначала удалите или перенесите их',
    'Failed to check items in this location': 'Не удалось проверить вещи в этом месте',
    'This location has items in it — move them elsewhere first': 'В этом месте есть вещи — сначала перенесите их в другое место',
    'Failed to delete location': 'Не удалось удалить место',
    'Location not found': 'Место не найдено',
    'does not look like a data URL': 'Не похоже на data URL',
    'invalid data URL: missing comma separator': 'Некорректный data URL: нет запятой-разделителя',
  },
  de: {
    'Authentication required': 'Authentifizierung erforderlich',
    'Invalid request body': 'Ungültiger Anfrageinhalt',
    'Please enter a username': 'Bitte einen Benutzernamen eingeben',
    'Username must be at least 3 characters': 'Der Benutzername muss mindestens 3 Zeichen lang sein',
    'Password must be at least 4 characters': 'Das Passwort muss mindestens 4 Zeichen lang sein',
    'Failed to process password': 'Passwort konnte nicht verarbeitet werden',
    'A user with this username is already registered': 'Ein Benutzer mit diesem Benutzernamen ist bereits registriert',
    'Failed to create user': 'Benutzer konnte nicht erstellt werden',
    'User created, but failed to start a session': 'Benutzer erstellt, aber die Sitzung konnte nicht gestartet werden',
    'Incorrect username or password': 'Falscher Benutzername oder falsches Passwort',
    'Failed to start a session': 'Sitzung konnte nicht gestartet werden',
    'Not authenticated': 'Nicht authentifiziert',
    'Unsupported language': 'Nicht unterstützte Sprache',
    'Failed to save language preference': 'Spracheinstellung konnte nicht gespeichert werden',
    'Failed to verify current password': 'Aktuelles Passwort konnte nicht überprüft werden',
    'Incorrect current password': 'Aktuelles Passwort ist falsch',
    'Failed to update username': 'Benutzername konnte nicht aktualisiert werden',
    'Failed to update password': 'Passwort konnte nicht aktualisiert werden',
    'Failed to load items': 'Gegenstände konnten nicht geladen werden',
    'Failed to read item': 'Gegenstand konnte nicht gelesen werden',
    'Failed to load photos': 'Fotos konnten nicht geladen werden',
    'Please check the form fields': 'Bitte die Formularfelder überprüfen',
    'Enter the item name': 'Geben Sie den Namen des Gegenstands ein',
    'Choose a location': 'Wählen Sie einen Ort',
    'Failed to verify location': 'Ort konnte nicht überprüft werden',
    'The specified location was not found': 'Der angegebene Ort wurde nicht gefunden',
    'Failed to save item': 'Gegenstand konnte nicht gespeichert werden',
    'Failed to save photos': 'Fotos konnten nicht gespeichert werden',
    'Item saved, but failed to read it back': 'Gegenstand gespeichert, konnte aber nicht erneut gelesen werden',
    'Record not found': 'Eintrag nicht gefunden',
    'Failed to update item': 'Gegenstand konnte nicht aktualisiert werden',
    'Failed to update photos': 'Fotos konnten nicht aktualisiert werden',
    'Item updated, but failed to read it back': 'Gegenstand aktualisiert, konnte aber nicht erneut gelesen werden',
    'Failed to delete item': 'Gegenstand konnte nicht gelöscht werden',
    'Failed to load locations': 'Orte konnten nicht geladen werden',
    'Failed to read location': 'Ort konnte nicht gelesen werden',
    'Please enter a location name': 'Bitte einen Namen für den Ort eingeben',
    'Failed to verify parent location': 'Übergeordneter Ort konnte nicht überprüft werden',
    'Parent location not found': 'Übergeordneter Ort nicht gefunden',
    'Failed to save location': 'Ort konnte nicht gespeichert werden',
    'Failed to check nested locations': 'Unterorte konnten nicht überprüft werden',
    'This location has nested locations — delete or move them first': 'Dieser Ort hat Unterorte — löschen oder verschieben Sie diese zuerst',
    'Failed to check items in this location': 'Gegenstände an diesem Ort konnten nicht überprüft werden',
    'This location has items in it — move them elsewhere first': 'An diesem Ort befinden sich Gegenstände — verschieben Sie diese zuerst woanders hin',
    'Failed to delete location': 'Ort konnte nicht gelöscht werden',
    'Location not found': 'Ort nicht gefunden',
    'does not look like a data URL': 'sieht nicht wie eine Daten-URL aus',
    'invalid data URL: missing comma separator': 'ungültige Daten-URL: Komma-Trennzeichen fehlt',
  },
  es: {
    'Authentication required': 'Se requiere autenticación',
    'Invalid request body': 'Cuerpo de la solicitud no válido',
    'Please enter a username': 'Introduce un nombre de usuario',
    'Username must be at least 3 characters': 'El nombre de usuario debe tener al menos 3 caracteres',
    'Password must be at least 4 characters': 'La contraseña debe tener al menos 4 caracteres',
    'Failed to process password': 'No se pudo procesar la contraseña',
    'A user with this username is already registered': 'Ya existe un usuario registrado con este nombre de usuario',
    'Failed to create user': 'No se pudo crear el usuario',
    'User created, but failed to start a session': 'Usuario creado, pero no se pudo iniciar la sesión',
    'Incorrect username or password': 'Nombre de usuario o contraseña incorrectos',
    'Failed to start a session': 'No se pudo iniciar la sesión',
    'Not authenticated': 'No autenticado',
    'Unsupported language': 'Idioma no admitido',
    'Failed to save language preference': 'No se pudo guardar la preferencia de idioma',
    'Failed to verify current password': 'No se pudo verificar la contraseña actual',
    'Incorrect current password': 'La contraseña actual es incorrecta',
    'Failed to update username': 'No se pudo actualizar el nombre de usuario',
    'Failed to update password': 'No se pudo actualizar la contraseña',
    'Failed to load items': 'No se pudieron cargar los objetos',
    'Failed to read item': 'No se pudo leer el objeto',
    'Failed to load photos': 'No se pudieron cargar las fotos',
    'Please check the form fields': 'Revisa los campos del formulario',
    'Enter the item name': 'Introduce el nombre del objeto',
    'Choose a location': 'Elige una ubicación',
    'Failed to verify location': 'No se pudo verificar la ubicación',
    'The specified location was not found': 'No se encontró la ubicación especificada',
    'Failed to save item': 'No se pudo guardar el objeto',
    'Failed to save photos': 'No se pudieron guardar las fotos',
    'Item saved, but failed to read it back': 'Objeto guardado, pero no se pudo volver a leer',
    'Record not found': 'Registro no encontrado',
    'Failed to update item': 'No se pudo actualizar el objeto',
    'Failed to update photos': 'No se pudieron actualizar las fotos',
    'Item updated, but failed to read it back': 'Objeto actualizado, pero no se pudo volver a leer',
    'Failed to delete item': 'No se pudo eliminar el objeto',
    'Failed to load locations': 'No se pudieron cargar las ubicaciones',
    'Failed to read location': 'No se pudo leer la ubicación',
    'Please enter a location name': 'Introduce un nombre para la ubicación',
    'Failed to verify parent location': 'No se pudo verificar la ubicación superior',
    'Parent location not found': 'Ubicación superior no encontrada',
    'Failed to save location': 'No se pudo guardar la ubicación',
    'Failed to check nested locations': 'No se pudieron comprobar las ubicaciones anidadas',
    'This location has nested locations — delete or move them first': 'Esta ubicación tiene ubicaciones anidadas — elimínalas o muévelas primero',
    'Failed to check items in this location': 'No se pudieron comprobar los objetos de esta ubicación',
    'This location has items in it — move them elsewhere first': 'Esta ubicación tiene objetos dentro — muévelos a otro lugar primero',
    'Failed to delete location': 'No se pudo eliminar la ubicación',
    'Location not found': 'Ubicación no encontrada',
    'does not look like a data URL': 'no parece una URL de datos',
    'invalid data URL: missing comma separator': 'URL de datos no válida: falta la coma separadora',
  },
  fr: {
    'Authentication required': 'Authentification requise',
    'Invalid request body': 'Corps de la requête invalide',
    'Please enter a username': "Saisissez un nom d'utilisateur",
    'Username must be at least 3 characters': "Le nom d'utilisateur doit comporter au moins 3 caractères",
    'Password must be at least 4 characters': 'Le mot de passe doit comporter au moins 4 caractères',
    'Failed to process password': 'Impossible de traiter le mot de passe',
    'A user with this username is already registered': "Un utilisateur portant ce nom d'utilisateur est déjà enregistré",
    'Failed to create user': "Impossible de créer l'utilisateur",
    'User created, but failed to start a session': "Utilisateur créé, mais impossible d'ouvrir une session",
    'Incorrect username or password': "Nom d'utilisateur ou mot de passe incorrect",
    'Failed to start a session': "Impossible d'ouvrir une session",
    'Not authenticated': 'Non authentifié',
    'Unsupported language': 'Langue non prise en charge',
    'Failed to save language preference': "Impossible d'enregistrer la préférence de langue",
    'Failed to verify current password': 'Impossible de vérifier le mot de passe actuel',
    'Incorrect current password': 'Mot de passe actuel incorrect',
    'Failed to update username': "Impossible de mettre à jour le nom d'utilisateur",
    'Failed to update password': 'Impossible de mettre à jour le mot de passe',
    'Failed to load items': 'Impossible de charger les objets',
    'Failed to read item': "Impossible de lire l'objet",
    'Failed to load photos': 'Impossible de charger les photos',
    'Please check the form fields': 'Vérifiez les champs du formulaire',
    'Enter the item name': "Saisissez le nom de l'objet",
    'Choose a location': 'Choisissez un emplacement',
    'Failed to verify location': "Impossible de vérifier l'emplacement",
    'The specified location was not found': "L'emplacement indiqué est introuvable",
    'Failed to save item': "Impossible d'enregistrer l'objet",
    'Failed to save photos': "Impossible d'enregistrer les photos",
    'Item saved, but failed to read it back': "Objet enregistré, mais impossible de le relire",
    'Record not found': 'Enregistrement introuvable',
    'Failed to update item': "Impossible de mettre à jour l'objet",
    'Failed to update photos': 'Impossible de mettre à jour les photos',
    'Item updated, but failed to read it back': 'Objet mis à jour, mais impossible de le relire',
    'Failed to delete item': "Impossible de supprimer l'objet",
    'Failed to load locations': 'Impossible de charger les emplacements',
    'Failed to read location': "Impossible de lire l'emplacement",
    'Please enter a location name': "Saisissez un nom pour l'emplacement",
    'Failed to verify parent location': "Impossible de vérifier l'emplacement parent",
    'Parent location not found': 'Emplacement parent introuvable',
    'Failed to save location': "Impossible d'enregistrer l'emplacement",
    'Failed to check nested locations': 'Impossible de vérifier les emplacements imbriqués',
    'This location has nested locations — delete or move them first': "Cet emplacement contient des emplacements imbriqués — supprimez-les ou déplacez-les d'abord",
    'Failed to check items in this location': 'Impossible de vérifier les objets de cet emplacement',
    'This location has items in it — move them elsewhere first': "Cet emplacement contient des objets — déplacez-les ailleurs d'abord",
    'Failed to delete location': "Impossible de supprimer l'emplacement",
    'Location not found': 'Emplacement introuvable',
    'does not look like a data URL': 'ne ressemble pas à une URL de données',
    'invalid data URL: missing comma separator': 'URL de données invalide : séparateur virgule manquant',
  },
};

// Regex-based translations for compound messages with a dynamic suffix,
// per locale (see localizeServerMessage below).
const SERVER_ERROR_PATTERNS = {
  ru: [
    { re: /^Invalid request body: request too large \(max (\d+) MB\)$/,
      fmt: m => `Некорректное тело запроса: слишком большой запрос (максимум ${m[1]} МБ)` },
    { re: /^photo #(\d+): failed to process \((.+)\)$/,
      fmt: (m, locale) => `Фото №${m[1]}: не удалось обработать (${localizeServerMessage(locale, m[2])})` },
  ],
  de: [
    { re: /^Invalid request body: request too large \(max (\d+) MB\)$/,
      fmt: m => `Ungültiger Anfrageinhalt: Anfrage zu groß (maximal ${m[1]} MB)` },
    { re: /^photo #(\d+): failed to process \((.+)\)$/,
      fmt: (m, locale) => `Foto Nr. ${m[1]}: konnte nicht verarbeitet werden (${localizeServerMessage(locale, m[2])})` },
  ],
  es: [
    { re: /^Invalid request body: request too large \(max (\d+) MB\)$/,
      fmt: m => `Cuerpo de la solicitud no válido: solicitud demasiado grande (máximo ${m[1]} MB)` },
    { re: /^photo #(\d+): failed to process \((.+)\)$/,
      fmt: (m, locale) => `Foto n.º ${m[1]}: no se pudo procesar (${localizeServerMessage(locale, m[2])})` },
  ],
  fr: [
    { re: /^Invalid request body: request too large \(max (\d+) MB\)$/,
      fmt: m => `Corps de la requête invalide : requête trop volumineuse (maximum ${m[1]} Mo)` },
    { re: /^photo #(\d+): failed to process \((.+)\)$/,
      fmt: (m, locale) => `Photo n° ${m[1]} : échec du traitement (${localizeServerMessage(locale, m[2])})` },
  ],
};

// Translates a backend error message into the current locale. Compound
// messages ("Invalid request body: ...", "photo #N: failed to process
// (...)") are translated piecewise, recursively; anything unrecognized
// (including messages with a dynamic, unpredictable suffix) is left as-is.
function localizeServerMessage(locale, message) {
  const table = SERVER_ERRORS[locale];
  if (!table || !message) return message;
  if (table[message]) return table[message];

  for (const { re, fmt } of SERVER_ERROR_PATTERNS[locale]) {
    const m = message.match(re);
    if (m) return fmt(m, locale);
  }

  return message;
}
