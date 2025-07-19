import { ConnectError } from '@connectrpc/connect';
import { Modal } from 'bootstrap';
import { getServerAddr } from './helpers';
import type { AppState, AppCommand, AppEvent } from './types.ts';
import { WebSocketCommunicator } from './websocket-communicator.ts';

enum StartState {
  NOT_STARTED = 'not_started',
  STARTING = 'starting',
  STARTED = 'started',
}

class Dashboard {
  private communicator: WebSocketCommunicator | null = null;
  private state: AppState = {
    source: {
      live: false,
      liveChangedAt: null,
      tracks: [],
      rtmpURL: '',
      rtmpsURL: '',
      container: {
        status: '',
        healthState: '',
        cpuPercent: 0,
        memoryUsageBytes: 0,
        rxRate: 0,
        txRate: 0,
        pullPercent: 0,
        pullStatus: '',
        pullProgress: '',
        imageName: '',
        err: null,
      },
    },
    destinations: [],
  };
  private isConnected = false;
  private destinationIdsToStartState: Map<string, StartState> = new Map();

  async init() {
    let serverAddr = getServerAddr();
    if (!serverAddr) {
      console.log('No server address configured.');
      window.location.href = '/login.html';
      return;
    }

    // TODO: connect directly with connectrpc if H2 is available.
    try {
      this.communicator = new WebSocketCommunicator({
        baseUrl: serverAddr,
        onEvent: (event) => this.handleEvent(event),
        onError: (error) => this.handleError(error),
        onUnauthorized: () => this.handleUnauthorized(),
        onConnectionStateChange: (connected) =>
          this.handleConnectionStateChange(connected),
      });

      // Connect to the backend via WebSocket
      await this.communicator.connect();

      // Setup UI event listeners
      this.setupEventListeners();

      console.log('Dashboard initialized successfully');
    } catch (error) {
      if (error instanceof ConnectError) {
        alert('Authentication failed: ' + error.message);
      } else {
        alert('Unexpected error: ' + error);
      }

      console.error('Dashboard initialization failed:', error);
    }
  }

  private handleError(error: Error) {
    console.error('Communication error:', error);
    this.showToast(`Connection error: ${error.message}`, 'error');
  }

  private handleUnauthorized() {
    console.error('Unauthorized access - redirecting to login');
    window.location.href = '/login.html';
  }

  private getStartState(destinationId: string): StartState {
    return (
      this.destinationIdsToStartState.get(destinationId) ||
      StartState.NOT_STARTED
    );
  }

  private containerStatusToStartState(containerStatus: string): StartState {
    switch (containerStatus) {
      case 'pulling':
      case 'created':
        return StartState.STARTING;
      case 'running':
      case 'restarting':
      case 'paused':
      case 'removing':
        return StartState.STARTED;
      default:
        return StartState.NOT_STARTED;
    }
  }

  private handleEvent(event: AppEvent) {
    switch (event.type) {
      case 'appStateChanged':
        this.state = event.state;

        for (const dest of this.state.destinations) {
          this.destinationIdsToStartState.set(
            dest.id,
            this.containerStatusToStartState(dest.container.status),
          );
        }

        this.updateUI();
        break;

      case 'handshakeCompleted':
        console.log('Handshake completed - ready to receive app state');
        break;

      case 'addDestinationFailed':
      case 'updateDestinationFailed':
        console.error('Destination operation failed:', event);
        this.handleSaveDestinationError(event.error);
        break;

      case 'destinationAdded':
        this.showToast('Destination added successfully', 'success');
        this.closeDestinationModal();
        break;
      case 'destinationUpdated':
        this.showToast('Destination updated successfully', 'success');
        this.closeDestinationModal();
        break;
      case 'destinationRemoved':
        this.showToast('Destination removed successfully', 'success');
        break;
      case 'destinationStarted':
        this.showToast('Destination started', 'success');
        break;
      case 'destinationStopped':
        this.showToast('Destination stopped', 'success');
        break;
      case 'destinationStreamExited':
        console.log(`Destination stream exited:`, event);

        if (this.destinationIdsToStartState.has(event.id)) {
          this.destinationIdsToStartState.set(event.id, StartState.NOT_STARTED);
        }

        this.showToast(
          `Stream to ${event.name} exited: ${event.error}`,
          'error',
        );
        break;

      case 'error': // fatal error
        console.error('Backend error:', event.message);
        this.showToast(event.message, 'error');
        break;

      default:
        console.log('Unhandled event type:', event);
    }
  }

  private handleConnectionStateChange(connected: boolean) {
    this.isConnected = connected;
    this.updateConnectionStatus();

    if (connected) {
      console.log('Connected to backend');
      this.showToast('Connected to backend', 'success');
    } else {
      console.log('Disconnected from backend');
      this.showToast('Disconnected from backend', 'error');
    }
  }

  private updateConnectionStatus() {
    const statusIndicator = document.getElementById('connection-status');
    if (statusIndicator) {
      statusIndicator.className = this.isConnected
        ? 'badge bg-success'
        : 'badge bg-danger';
      statusIndicator.innerHTML = this.isConnected
        ? '<i class="bi bi-wifi me-1"></i>Connected'
        : '<i class="bi bi-wifi-off me-1"></i>Disconnected';
    }
  }

  private updateUI() {
    this.updateSourcePanel();
    this.updateDestinationsTable();
    this.updateSourceURLs();
  }

  private updateSourcePanel() {
    const { source } = this.state;

    // Update source status
    const statusEl = document.getElementById('source-status');
    if (statusEl) {
      if (source.live) {
        statusEl.innerHTML =
          '<span class="badge bg-success fs-6"><i class="bi bi-broadcast me-1"></i>Receiving</span>';
      } else if (
        source.container.status === 'running' &&
        source.container.healthState === 'healthy'
      ) {
        statusEl.innerHTML =
          '<span class="badge bg-warning fs-6"><i class="bi bi-clock me-1"></i>Waiting</span>';
      } else {
        statusEl.innerHTML =
          '<span class="badge bg-danger fs-6"><i class="bi bi-exclamation-triangle me-1"></i>Not ready</span>';
      }
    }

    // Update tracks
    const tracksEl = document.getElementById('source-tracks');
    if (tracksEl) {
      tracksEl.textContent =
        source.tracks.length > 0 ? source.tracks.join(', ') : '—';
    }

    // Update health
    const healthEl = document.getElementById('source-health');
    if (healthEl) {
      healthEl.textContent = source.container.healthState || '—';
    }

    // Update CPU
    const cpuEl = document.getElementById('source-cpu');
    if (cpuEl) {
      cpuEl.textContent =
        source.container.status === 'running'
          ? Number(source.container.cpuPercent).toFixed(1)
          : '—';
    }

    // Update Memory
    const memEl = document.getElementById('source-mem');
    if (memEl) {
      memEl.textContent =
        source.container.status === 'running'
          ? (Number(source.container.memoryUsageBytes) / 1024 / 1024).toFixed(1)
          : '—';
    }

    // Update Rx Rate
    const rxEl = document.getElementById('source-rx');
    if (rxEl) {
      rxEl.textContent =
        source.container.status === 'running'
          ? Number(source.container.rxRate).toString()
          : '—';
    }
  }

  private handleDestinationClick(event: MouseEvent) {
    const target = (event.target as HTMLElement).closest('button');
    if (!target) {
      return;
    }
    const destinationId = target.closest('tr')?.dataset.destinationId;
    if (!destinationId) {
      console.warn('No destination ID found in target');
      return;
    }

    switch (target.dataset.buttonType) {
      case 'start':
        this.startDestination(destinationId);
        break;
      case 'stop':
        this.stopDestination(destinationId);
        break;
      case 'edit':
        this.editDestination(destinationId);
        break;
      case 'remove':
        this.removeDestination(destinationId);
        break;
    }
  }

  private updateDestinationsTable() {
    const tbody = document.getElementById('destinations-tbody');
    if (!tbody) return;

    const noDestRow = document.getElementById('no-destinations-row');

    if (this.state.destinations.length === 0) {
      if (noDestRow) noDestRow.style.display = '';
      return;
    }

    if (noDestRow) noDestRow.style.display = 'none';

    // First, remove any rows that no longer have a corresponding destination.
    for (const row of tbody.querySelectorAll('tr[data-destination-id]')) {
      const destId = (row as HTMLTableRowElement).dataset.destinationId;
      if (!this.state.destinations.some((d) => d.id === destId)) {
        row.remove();
      }
    }

    // Add or update rows for each destination.
    this.state.destinations.forEach((dest) => {
      // Check if the row exists, and re-use it if so.
      // This is to try to avoid dropped events when the table is updated.
      let row = tbody.querySelector(
        "tr[data-destination-id='" + dest.id + "']",
      ) as HTMLTableRowElement | null;

      const isLive = this.getStartState(dest.id) !== StartState.NOT_STARTED;
      const canStart = this.state.source.live && !isLive;

      if (!row) {
        row = document.createElement('tr') as HTMLTableRowElement;
        row.dataset.destinationId = dest.id;
        row.innerHTML = `
        <td></td>
        <td></td>
        <td class="d-none d-md-table-cell"></td>
        <td class="d-none d-lg-table-cell"></td>
        <td class="d-none d-lg-table-cell"></td>
        <td class="d-none d-lg-table-cell"></td>
        <td class="d-none d-xl-table-cell"></td>
        <td>
          <div class="destination-actions">
            <button type="button" class="btn btn-success" data-button-type="start"><i class="bi bi-play-fill me-1"></i>Start</button>
            <button type="button" class="btn btn-secondary d-none d-sm-inline-block" data-button-type="edit"><i class="bi bi-pencil me-1"></i>Edit</button>
            <button type="button" class="btn btn-remove" title="Remove destination" data-button-type="remove"><i class="bi bi-trash"></i></button>
          </div>
        </td>
      `;
        tbody.appendChild(row);
      }

      row.querySelector('td:first-child')!.textContent = this.escapeHtml(
        dest.name,
      );

      const statusEl = isLive
        ? '<span class="badge bg-success"><i class="bi bi-play-fill me-1"></i>Sending</span>'
        : dest.status === 'starting'
          ? '<span class="badge bg-warning"><i class="bi bi-arrow-clockwise me-1"></i>Starting</span>'
          : '<span class="badge bg-secondary"><i class="bi bi-stop-fill me-1"></i>Off-air</span>';
      row.querySelector('td:nth-child(2)')!.innerHTML = statusEl;

      row.querySelector('td:nth-child(3)')!.textContent =
        dest.container.status || '—';

      row.querySelector('td:nth-child(4)')!.textContent =
        dest.status === 'live' ? 'healthy' : '—';

      row.querySelector('td:nth-child(5)')!.textContent =
        dest.container.status === 'running'
          ? Number(dest.container.cpuPercent).toFixed(1)
          : '—';

      row.querySelector('td:nth-child(6)')!.textContent =
        dest.container.status === 'running'
          ? (Number(dest.container.memoryUsageBytes) / 1000 / 1000).toFixed(1)
          : '—';

      row.querySelector('td:nth-child(7)')!.textContent =
        dest.container.status === 'running'
          ? Number(dest.container.txRate).toString()
          : '—';

      const startStopButton = row.querySelector(
        '.destination-actions button:first-child',
      ) as HTMLButtonElement;
      if (isLive) {
        startStopButton.textContent = 'Stop';
        startStopButton.className = 'btn btn-danger';
        startStopButton.innerHTML = `<i class="bi bi-stop-fill me-1"></i>Stop`;
        startStopButton.dataset.buttonType = 'stop';
      } else if (canStart) {
        startStopButton.textContent = 'Start';
        startStopButton.disabled = false;
        startStopButton.className = 'btn btn-success';
        startStopButton.innerHTML = `<i class="bi bi-play fill me-1"></i>Start`;
        startStopButton.dataset.buttonType = 'start';
      } else {
        startStopButton.textContent = 'Start';
        startStopButton.className = 'btn btn-secondary';
        startStopButton.innerHTML = `<i class="bi bi-play fill me-1"></i>Start`;
        startStopButton.disabled = true;
        startStopButton.dataset.buttonType = 'start';
      }

      const editButton = row.querySelector(
        '.destination-actions button:nth-child(2)',
      ) as HTMLButtonElement;
      editButton.disabled = isLive;
    });
  }

  private updateSourceURLs() {
    const { source } = this.state;

    const rtmpSection = document.getElementById('rtmp-url-section');
    const rtmpsSection = document.getElementById('rtmps-url-section');
    const noUrlsMessage = document.getElementById('no-urls-message');
    const rtmpInput = document.getElementById(
      'rtmp-url-input',
    ) as HTMLInputElement;
    const rtmpsInput = document.getElementById(
      'rtmps-url-input',
    ) as HTMLInputElement;

    let hasAnyUrls = false;

    // Handle RTMP URL
    if (source.rtmpURL && rtmpSection && rtmpInput) {
      rtmpSection.style.display = 'block';
      rtmpInput.value = source.rtmpURL;
      hasAnyUrls = true;
    } else if (rtmpSection) {
      rtmpSection.style.display = 'none';
    }

    // Handle RTMPS URL
    if (source.rtmpsURL && rtmpsSection && rtmpsInput) {
      rtmpsSection.style.display = 'block';
      rtmpsInput.value = source.rtmpsURL;
      hasAnyUrls = true;
    } else if (rtmpsSection) {
      rtmpsSection.style.display = 'none';
    }

    // Show/hide "no URLs" message
    if (noUrlsMessage) {
      noUrlsMessage.style.display = hasAnyUrls ? 'none' : 'block';
    }
  }

  private escapeHtml(text: string): string {
    const div = document.createElement('div');
    div.textContent = text;
    return div.innerHTML;
  }

  private setupEventListeners() {
    // Add destination button
    const addBtn = document.getElementById('add-destination-btn');
    addBtn?.addEventListener('click', () => this.addDestination());

    // Save destination button
    const saveBtn = document.getElementById('save-destination-btn');
    saveBtn?.addEventListener('click', () => this.saveDestination());

    const tbody = document.getElementById('destinations-tbody');
    tbody?.addEventListener('click', (e) => {
      this.handleDestinationClick(e);
    });

    // Copy URL buttons
    const copyRtmpBtn = document.getElementById('copy-rtmp-btn');
    copyRtmpBtn?.addEventListener('click', () => this.copyToClipboard('rtmp'));

    const copyRtmpsBtn = document.getElementById('copy-rtmps-btn');
    copyRtmpsBtn?.addEventListener('click', () =>
      this.copyToClipboard('rtmps'),
    );

    // Logout button
    const logoutLink = document.getElementById('logout-link');
    logoutLink?.addEventListener('click', (e) => {
      e.preventDefault();
      this.showLogoutConfirmation();
    });

    // Logout confirmation button
    const logoutConfirmBtn = document.getElementById('logout-confirm-btn');
    logoutConfirmBtn?.addEventListener('click', () => this.performLogout());
  }

  private async copyToClipboard(urlType: 'rtmp' | 'rtmps') {
    const url =
      urlType === 'rtmp'
        ? this.state.source.rtmpURL
        : this.state.source.rtmpsURL;
    if (!url) return;

    try {
      await navigator.clipboard.writeText(url);
      this.showToast(
        `${urlType.toUpperCase()} URL copied to clipboard`,
        'success',
      );
    } catch (err) {
      console.error('Failed to copy to clipboard:', err);
      this.showToast('Failed to copy URL', 'error');
    }
  }

  private showToast(message: string, type: 'success' | 'error') {
    const toast = document.createElement('div');
    toast.className = `alert alert-${type === 'success' ? 'success' : 'danger'} position-fixed toast-notification`;

    // Calculate position based on existing toasts
    const existingToasts = document.querySelectorAll('.toast-notification');
    const topOffset = 80 + existingToasts.length * 70; // 70px spacing between toasts

    toast.style.top = `${topOffset}px`;
    toast.style.right = '20px';
    toast.style.zIndex = '9999';
    toast.style.maxWidth = '400px';
    toast.style.borderRadius = '8px';
    toast.style.boxShadow = '0 4px 12px rgba(0, 0, 0, 0.3)';
    toast.style.transition = 'all 0.3s ease-in-out';
    toast.textContent = message;

    document.body.appendChild(toast);

    // Auto-remove and reposition remaining toasts
    setTimeout(() => {
      toast.style.opacity = '0';
      toast.style.transform = 'translateX(100%)';
      setTimeout(() => {
        toast.remove();
        this.repositionToasts();
      }, 300);
    }, 3000);
  }

  private repositionToasts() {
    const toasts = document.querySelectorAll('.toast-notification');
    toasts.forEach((toast, index) => {
      const element = toast as HTMLElement;
      element.style.top = `${80 + index * 70}px`;
    });
  }

  private showLogoutConfirmation() {
    const logoutModal = document.getElementById('logout-modal');
    if (logoutModal) {
      const modal = new Modal(logoutModal);
      modal.show();
    }
  }

  private performLogout() {
    const baseUrl = getServerAddr();
    if (!baseUrl) {
      window.location.href = '/login.html';
      return;
    }

    // Create a form and submit it to logout
    const form = document.createElement('form');
    form.method = 'POST';
    form.action = new URL('session/destroy', baseUrl).href;
    document.body.appendChild(form);
    form.submit();
  }

  // Public methods called by onclick handlers
  addDestination() {
    this.openDestinationModal();
  }

  async startDestination(destinationId: string) {
    if (this.getStartState(destinationId) !== StartState.NOT_STARTED) {
      return;
    }

    console.log('Start destination:', destinationId);
    await this.sendCommand({
      type: 'startDestination',
      id: destinationId,
    });
  }

  async stopDestination(destinationId: string) {
    if (this.getStartState(destinationId) === StartState.NOT_STARTED) {
      return;
    }

    console.log('Stop destination:', destinationId);
    await this.sendCommand({
      type: 'stopDestination',
      id: destinationId,
    });
  }

  editDestination(destinationId: string) {
    const destination = this.state.destinations.find(
      (d) => d.id === destinationId,
    );
    if (destination) {
      this.openDestinationModal(destination);
    }
  }

  async removeDestination(destinationId: string) {
    const destination = this.state.destinations.find(
      (d) => d.id === destinationId,
    );
    if (!destination) {
      return;
    }

    const started =
      this.getStartState(destinationId) !== StartState.NOT_STARTED;

    const message = started
      ? 'This destination is currently live. Force remove?'
      : 'Are you sure you want to remove this destination?';

    const confirmed = await this.showConfirmModal(
      'Remove Destination',
      message,
    );
    if (!confirmed) return;

    await this.sendCommand({
      type: 'removeDestination',
      id: destinationId,
      force: started,
    });
  }

  private async sendCommand(command: AppCommand) {
    if (!this.communicator) {
      console.error('Communicator not initialized');
      this.showToast('Not connected to backend', 'error');
      return;
    }

    try {
      await this.communicator.sendCommand(command);
      console.log('Command sent:', command);
    } catch (error) {
      console.error('Failed to send command:', error);
      this.showToast('Failed to send command', 'error');
    }
  }

  cleanup() {
    if (this.communicator) {
      this.communicator.disconnect();
    }
  }

  private openDestinationModal(destination?: any) {
    const modal = document.getElementById('destination-modal');
    const modalTitle = document.getElementById('destination-modal-label');
    const form = document.getElementById('destination-form') as HTMLFormElement;

    if (!modal || !modalTitle || !form) return;

    // Set modal title
    modalTitle.textContent = destination
      ? 'Edit Destination'
      : 'Add Destination';

    // Clear validation classes
    form.classList.remove('was-validated');
    form.querySelectorAll('.form-control').forEach((control) => {
      control.classList.remove('is-invalid');
    });
    this.clearSaveDestinationError();

    // Populate form if editing
    if (destination) {
      (form.elements.namedItem('name') as HTMLInputElement).value =
        destination.name || '';
      (form.elements.namedItem('url') as HTMLInputElement).value =
        destination.url || '';
    } else {
      // Reset form for new destination
      form.reset();
    }

    // Store destination ID for editing
    form.dataset.destinationId = destination?.id || '';

    // Show modal
    const bootstrapModal = new Modal(modal);
    bootstrapModal.show();
  }

  // handleSaveDestinationError handles a server error when saving a destination.
  //
  // TODO: improve error handling consistency between client-side and
  // server-side errors.
  private handleSaveDestinationError(msg: string) {
    const form = document.getElementById('destination-form') as HTMLFormElement;

    if (!form) return;

    const errElement = form.querySelector(
      '#destination-error',
    ) as HTMLDivElement;
    errElement.style.display = 'block';

    (errElement.querySelector('p') as HTMLParagraphElement).textContent = msg;
  }

  private clearSaveDestinationError() {
    const form = document.getElementById('destination-form') as HTMLFormElement;

    if (!form) return;

    (form.querySelector('#destination-error') as HTMLDivElement).style.display =
      'none';
  }

  private validateForm(form: HTMLFormElement): boolean {
    const name = (
      form.elements.namedItem('name') as HTMLInputElement
    ).value.trim();
    const url = (
      form.elements.namedItem('url') as HTMLInputElement
    ).value.trim();

    let valid = true;

    // Validate name
    const nameInput = form.elements.namedItem('name') as HTMLInputElement;
    if (!name) {
      nameInput.classList.add('is-invalid');
      valid = false;
    } else {
      nameInput.classList.remove('is-invalid');
    }

    // Validate URL
    const urlInput = form.elements.namedItem('url') as HTMLInputElement;
    if (!url) {
      urlInput.classList.add('is-invalid');
      return false;
    }

    try {
      new URL(url);
      urlInput.classList.remove('is-invalid');
    } catch {
      urlInput.classList.add('is-invalid');
      return false;
    }

    return valid;
  }

  private async saveDestination() {
    const form = document.getElementById('destination-form') as HTMLFormElement;
    if (!form) return;

    this.clearSaveDestinationError();

    if (!this.validateForm(form)) {
      form.classList.add('was-validated');
      return;
    }

    const formData = new FormData(form);
    const destinationId = form.dataset.destinationId;

    const isUpdate = !!destinationId;
    if (
      isUpdate &&
      this.destinationIdsToStartState.get(destinationId) !==
        StartState.NOT_STARTED
    ) {
      this.showToast(
        'You cannot update a destination while it is live.',
        'error',
      );
      return;
    }

    const data = {
      name: formData.get('name') as string,
      url: formData.get('url') as string,
    };

    try {
      if (isUpdate) {
        await this.sendCommand({
          type: 'updateDestination',
          id: destinationId,
          ...data,
        });
      } else {
        await this.sendCommand({
          type: 'addDestination',
          ...data,
        });
      }
    } catch (error) {
      this.showToast('Failed to save destination', 'error');
      console.error('Error saving destination:', error);
    }
  }

  private closeDestinationModal() {
    const modal = document.getElementById('destination-modal');
    if (modal) {
      const bootstrapModal = Modal.getInstance(modal);
      if (bootstrapModal) {
        bootstrapModal.hide();
      }
    }
  }

  private showConfirmModal(title: string, message: string): Promise<boolean> {
    return new Promise((resolve) => {
      const modal = document.getElementById('confirm-modal');
      const modalTitle = document.getElementById('confirm-modal-label');
      const modalMessage = document.getElementById('confirm-modal-message');
      const actionButton = document.getElementById('confirm-modal-action');

      if (!modal || !modalTitle || !modalMessage || !actionButton) {
        resolve(false);
        return;
      }

      // Set modal content
      modalTitle.textContent = title;
      modalMessage.textContent = message;

      // Remove existing event listeners
      const newActionButton = actionButton.cloneNode(true) as HTMLElement;
      actionButton.parentNode?.replaceChild(newActionButton, actionButton);

      // Add event listeners
      const handleConfirm = () => {
        bootstrapModal.hide();
        resolve(true);
      };

      const handleCancel = () => {
        bootstrapModal.hide();
        resolve(false);
      };

      newActionButton.addEventListener('click', handleConfirm);

      // Show modal
      const bootstrapModal = new Modal(modal);

      // Handle modal close events
      modal.addEventListener('hidden.bs.modal', handleCancel, { once: true });

      bootstrapModal.show();
    });
  }
}

// Initialize dashboard when DOM is ready
let dashboard: Dashboard | null = null;

document.addEventListener('DOMContentLoaded', async () => {
  dashboard = new Dashboard();
  await dashboard.init();

  // Expose dashboard globally for onclick handlers
  (window as any).dashboard = dashboard;
});

// Cleanup on page unload
window.addEventListener('beforeunload', () => {
  if (dashboard) {
    dashboard.cleanup();
  }
});
