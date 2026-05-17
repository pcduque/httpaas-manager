-- Banco "Andes Capital" — esquema demo para la DBaaS.
-- Compatible con MariaDB y PostgreSQL: usa IDs explícitos para evitar la
-- divergencia AUTO_INCREMENT (MariaDB) vs SERIAL/IDENTITY (PostgreSQL).

CREATE TABLE clientes (
    id              INT PRIMARY KEY,
    cedula          VARCHAR(20) NOT NULL UNIQUE,
    nombre          VARCHAR(120) NOT NULL,
    email           VARCHAR(120) NOT NULL UNIQUE,
    telefono        VARCHAR(20),
    ciudad          VARCHAR(60),
    fecha_registro  DATE NOT NULL
);

CREATE TABLE cuentas (
    id              INT PRIMARY KEY,
    cliente_id      INT NOT NULL REFERENCES clientes(id),
    numero          VARCHAR(20) NOT NULL UNIQUE,
    tipo            VARCHAR(20) NOT NULL,           -- ahorros | corriente
    saldo           DECIMAL(14,2) NOT NULL DEFAULT 0,
    moneda          CHAR(3) NOT NULL DEFAULT 'COP',
    abierta_en      DATE NOT NULL,
    activa          INT NOT NULL DEFAULT 1          -- 0 = cerrada, 1 = activa
);

CREATE TABLE transacciones (
    id              INT PRIMARY KEY,
    cuenta_id       INT NOT NULL REFERENCES cuentas(id),
    tipo            VARCHAR(20) NOT NULL,           -- deposito | retiro | transferencia | pago
    monto           DECIMAL(14,2) NOT NULL,
    descripcion     VARCHAR(200),
    cuenta_destino  INT REFERENCES cuentas(id),
    ocurrida_en     TIMESTAMP NOT NULL
);

CREATE TABLE creditos (
    id              INT PRIMARY KEY,
    cliente_id      INT NOT NULL REFERENCES clientes(id),
    monto           DECIMAL(14,2) NOT NULL,
    plazo_meses     INT NOT NULL,
    tasa_anual      DECIMAL(5,2) NOT NULL,
    estado          VARCHAR(20) NOT NULL,           -- aprobado | en_revision | rechazado | pagado
    otorgado_en     DATE NOT NULL
);

-- Clientes
INSERT INTO clientes (id, cedula, nombre, email, telefono, ciudad, fecha_registro) VALUES
    (1, '1098765432', 'Andrea Lopez Restrepo',  'andrea.lopez@andescap.co', '+57 300 555 0142', 'Armenia',   '2023-04-12'),
    (2, '1023456789', 'Bruno Mejia Vargas',     'bruno.mejia@andescap.co',  '+57 311 555 0098', 'Pereira',   '2023-07-30'),
    (3, '1187654321', 'Carolina Trujillo Diaz', 'caro.trujillo@andescap.co','+57 320 555 0011', 'Manizales', '2024-02-04'),
    (4, '1145678901', 'Diego Patino Suarez',    'diego.patino@andescap.co', '+57 301 555 0467', 'Armenia',   '2024-08-19'),
    (5, '1209876543', 'Elena Cardona Rios',     'elena.cardona@andescap.co','+57 314 555 0820', 'Cali',      '2025-01-22');

-- Cuentas
INSERT INTO cuentas (id, cliente_id, numero, tipo, saldo, moneda, abierta_en, activa) VALUES
    (101, 1, '4001-0000-0001', 'ahorros',   3250000.00, 'COP', '2023-04-12', 1),
    (102, 1, '4001-0000-0002', 'corriente',  812000.50, 'COP', '2024-09-01', 1),
    (103, 2, '4001-0000-0003', 'ahorros',  15800000.00, 'COP', '2023-07-30', 1),
    (104, 3, '4001-0000-0004', 'ahorros',     45000.00, 'COP', '2024-02-04', 1),
    (105, 4, '4001-0000-0005', 'corriente', 2100000.00, 'COP', '2024-08-19', 1),
    (106, 5, '4001-0000-0006', 'ahorros',    980000.75, 'COP', '2025-01-22', 1),
    (107, 5, '4001-0000-0007', 'corriente',       0.00, 'COP', '2025-03-15', 0);

-- Transacciones
INSERT INTO transacciones (id, cuenta_id, tipo, monto, descripcion, cuenta_destino, ocurrida_en) VALUES
    (5001, 101, 'deposito',      1500000.00, 'Consignacion nomina abril',   NULL, '2026-04-30 09:14:22'),
    (5002, 101, 'retiro',         200000.00, 'Cajero centro comercial',     NULL, '2026-05-02 18:42:10'),
    (5003, 103, 'deposito',      4200000.00, 'Pago factura proveedor',      NULL, '2026-05-05 11:05:33'),
    (5004, 103, 'transferencia',  500000.00, 'Pago arriendo',                101, '2026-05-06 08:30:00'),
    (5005, 105, 'pago',           150000.00, 'Servicio energia EPSA',       NULL, '2026-05-07 16:11:45'),
    (5006, 106, 'deposito',       320000.00, 'Recarga billetera',           NULL, '2026-05-10 12:00:00'),
    (5007, 102, 'transferencia',   80000.00, 'Pago Spotify familiar',       NULL, '2026-05-11 21:00:00'),
    (5008, 104, 'deposito',       250000.00, 'Devolucion impuestos',        NULL, '2026-05-13 10:25:18');

-- Creditos
INSERT INTO creditos (id, cliente_id, monto, plazo_meses, tasa_anual, estado, otorgado_en) VALUES
    (9001, 1, 12000000.00, 36, 18.50, 'aprobado',     '2024-05-10'),
    (9002, 2, 35000000.00, 60, 14.25, 'aprobado',     '2024-01-18'),
    (9003, 3,  2500000.00, 12, 22.00, 'pagado',       '2024-03-22'),
    (9004, 4,  8000000.00, 24, 19.75, 'en_revision',  '2026-05-01'),
    (9005, 5,  4500000.00, 18, 21.30, 'rechazado',    '2026-04-15');
